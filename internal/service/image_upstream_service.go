package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"web2api/internal/dto"
	"web2api/internal/model"
)

type InputImage struct {
	Data     []byte
	FileName string
	MimeType string
}

type ImageUpstreamService struct {
	accountService *AccountService
	tlsVerify      bool
}

func NewImageUpstreamService(accountService *AccountService, tlsVerify bool) *ImageUpstreamService {
	return &ImageUpstreamService{
		accountService: accountService,
		tlsVerify:      tlsVerify,
	}
}

func (s *ImageUpstreamService) Generate(
	ctx context.Context,
	accessToken string,
	prompt string,
	model string,
) (dto.ImageDataItem, error) {
	item, err := s.accountService.GetByToken(ctx, accessToken)
	if err != nil {
		return dto.ImageDataItem{}, err
	}
	if item == nil {
		return dto.ImageDataItem{}, notFound("account not found")
	}

	session, err := s.newSession(ctx, item)
	if err != nil {
		return dto.ImageDataItem{}, err
	}

	conversationModel := s.resolveConversationModel(item, model)
	fileIDs, _, err := s.sendConversation(
		ctx,
		session,
		conversationModel,
		prompt,
		nil,
	)
	if err != nil {
		return dto.ImageDataItem{}, err
	}

	return s.downloadImage(ctx, session, fileIDs)
}

func (s *ImageUpstreamService) Edit(
	ctx context.Context,
	accessToken string,
	prompt string,
	model string,
	images []InputImage,
) (dto.ImageDataItem, error) {
	item, err := s.accountService.GetByToken(ctx, accessToken)
	if err != nil {
		return dto.ImageDataItem{}, err
	}
	if item == nil {
		return dto.ImageDataItem{}, notFound("account not found")
	}
	if len(images) == 0 {
		return dto.ImageDataItem{}, badRequest("image is required")
	}

	session, err := s.newSession(ctx, item)
	if err != nil {
		return dto.ImageDataItem{}, err
	}

	parts := make([]map[string]any, 0, len(images)+1)
	for _, image := range images {
		fileID, width, height, err := s.uploadImage(ctx, session, image)
		if err != nil {
			return dto.ImageDataItem{}, err
		}

		parts = append(parts, map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": "file-service://" + fileID,
			"size_bytes":    len(image.Data),
			"width":         width,
			"height":        height,
		})
	}
	parts = append(parts, map[string]any{
		"content_type": "text",
		"text":         prompt,
	})

	conversationModel := s.resolveConversationModel(item, model)
	fileIDs, _, err := s.sendConversation(
		ctx,
		session,
		conversationModel,
		prompt,
		parts,
	)
	if err != nil {
		return dto.ImageDataItem{}, err
	}

	return s.downloadImage(ctx, session, fileIDs)
}

type upstreamSession struct {
	client            *http.Client
	headers           map[string]string
	requirementsToken string
	proofToken        string
}

func (s *ImageUpstreamService) newSession(ctx context.Context, account any) (*upstreamSession, error) {
	item, ok := account.(*model.Account)
	if !ok {
		return nil, badGateway("invalid account")
	}

	client := newHTTPClient(s.tlsVerify, 120*time.Second)

	pageReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/", nil)
	if err != nil {
		return nil, err
	}
	pageReq.Header.Set("User-Agent", firstNonEmpty(item.UserAgent, defaultUserAgent))
	pageResp, err := client.Do(pageReq)
	if err == nil && pageResp.StatusCode < http.StatusBadRequest {
		body, _ := io.ReadAll(pageResp.Body)
		pageResp.Body.Close()
		updateProofBuildInfoFromHTML(string(body))
	}

	headers := s.accountService.accountHeaders(item)
	headers["Accept"] = "text/event-stream"

	requirementsToken, proofToken, err := s.fetchRequirements(ctx, client, headers)
	if err != nil {
		return nil, err
	}

	return &upstreamSession{
		client:            client,
		headers:           headers,
		requirementsToken: requirementsToken,
		proofToken:        proofToken,
	}, nil
}

func (s *ImageUpstreamService) fetchRequirements(
	ctx context.Context,
	client *http.Client,
	headers map[string]string,
) (string, string, error) {
	reqHeaders := cloneHeaders(headers)
	reqHeaders["Accept"] = "application/json"
	reqHeaders["OpenAI-Sentinel-Chat-Requirements-Token"] = getRequirementsToken(headers["User-Agent"])

	req, err := newJSONRequest(
		ctx,
		http.MethodPost,
		"https://chatgpt.com/backend-api/sentinel/chat-requirements",
		reqHeaders,
		map[string]any{},
	)
	if err != nil {
		return "", "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		snippet := readResponseSnippet(resp, 512)
		if resp.StatusCode == http.StatusUnauthorized {
			return "", "", unauthorized(firstNonEmpty(snippet, "invalid access token"))
		}
		return "", "", badGateway(firstNonEmpty(snippet, resp.Status))
	}

	payload := map[string]any{}
	if err := decodeJSONResponse(resp, &payload); err != nil {
		return "", "", err
	}

	token := asString(payload["token"])
	proofToken := ""
	if proof, ok := payload["proofofwork"].(map[string]any); ok {
		if required, _ := proof["required"].(bool); required {
			proofToken = generateProofToken(
				asString(proof["seed"]),
				asString(proof["difficulty"]),
				headers["User-Agent"],
			)
		}
	}

	return token, proofToken, nil
}

func (s *ImageUpstreamService) sendConversation(
	ctx context.Context,
	session *upstreamSession,
	model string,
	prompt string,
	multimodalParts []map[string]any,
) ([]string, string, error) {
	content := map[string]any{
		"content_type": "text",
		"parts":        []string{prompt},
	}
	if len(multimodalParts) > 0 {
		content = map[string]any{
			"content_type": "multimodal_text",
			"parts":        multimodalParts,
		}
	}

	body := map[string]any{
		"action":                        "next",
		"messages":                      []any{buildConversationMessage(content)},
		"model":                         model,
		"parent_message_id":             newUUID(),
		"history_and_training_disabled": false,
		"conversation_mode":             map[string]any{"kind": "primary_assistant"},
		"websocket_request_id":          newUUID(),
		"client_contextual_info":        buildClientContext(),
		"supports_buffering":            true,
	}

	headers := cloneHeaders(session.headers)
	headers["Accept"] = "text/event-stream"
	headers["OpenAI-Sentinel-Chat-Requirements-Token"] = session.requirementsToken
	if session.proofToken != "" {
		headers["OpenAI-Sentinel-Proof-Token"] = session.proofToken
	}

	req, err := newJSONRequest(
		ctx,
		http.MethodPost,
		"https://chatgpt.com/backend-api/conversation",
		headers,
		body,
	)
	if err != nil {
		return nil, "", err
	}

	resp, err := session.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		snippet := readResponseSnippet(resp, 1024)
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, "", unauthorized(firstNonEmpty(snippet, "invalid access token"))
		}
		return nil, "", badGateway(firstNonEmpty(snippet, resp.Status))
	}

	return parseSSE(resp)
}

func (s *ImageUpstreamService) uploadImage(
	ctx context.Context,
	session *upstreamSession,
	image InputImage,
) (string, int, int, error) {
	width, height := imageSize(image.Data)
	body := map[string]any{
		"file_name": image.FileName,
		"file_size": len(image.Data),
		"use_case":  "multimodal",
		"mime_type": image.MimeType,
	}

	headers := cloneHeaders(session.headers)
	headers["Accept"] = "application/json"
	headers["OpenAI-Sentinel-Chat-Requirements-Token"] = session.requirementsToken
	if session.proofToken != "" {
		headers["OpenAI-Sentinel-Proof-Token"] = session.proofToken
	}

	req, err := newJSONRequest(ctx, http.MethodPost, "https://chatgpt.com/backend-api/files", headers, body)
	if err != nil {
		return "", 0, 0, err
	}

	resp, err := session.client.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		snippet := readResponseSnippet(resp, 512)
		return "", 0, 0, badGateway(firstNonEmpty(snippet, resp.Status))
	}

	initPayload := map[string]any{}
	if err := decodeJSONResponse(resp, &initPayload); err != nil {
		return "", 0, 0, err
	}

	fileID := asString(initPayload["file_id"])
	uploadURL := asString(initPayload["upload_url"])
	if fileID == "" || uploadURL == "" {
		return "", 0, 0, badGateway("invalid upload response")
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(image.Data))
	if err != nil {
		return "", 0, 0, err
	}
	putReq.Header.Set("Content-Type", image.MimeType)
	putResp, err := session.client.Do(putReq)
	if err != nil {
		return "", 0, 0, err
	}
	if putResp.StatusCode >= http.StatusBadRequest {
		snippet := readResponseSnippet(putResp, 512)
		return "", 0, 0, badGateway(firstNonEmpty(snippet, putResp.Status))
	}
	putResp.Body.Close()

	finalReq, err := newJSONRequest(
		ctx,
		http.MethodPost,
		"https://chatgpt.com/backend-api/files/"+url.PathEscape(fileID)+"/uploaded",
		headers,
		map[string]any{},
	)
	if err != nil {
		return "", 0, 0, err
	}
	finalResp, err := session.client.Do(finalReq)
	if err != nil {
		return "", 0, 0, err
	}
	if finalResp.StatusCode >= http.StatusBadRequest {
		snippet := readResponseSnippet(finalResp, 512)
		return "", 0, 0, badGateway(firstNonEmpty(snippet, finalResp.Status))
	}
	finalResp.Body.Close()

	return fileID, width, height, nil
}

func (s *ImageUpstreamService) downloadImage(
	ctx context.Context,
	session *upstreamSession,
	fileIDs []string,
) (dto.ImageDataItem, error) {
	if len(fileIDs) == 0 {
		return dto.ImageDataItem{}, badGateway("upstream returned no image file")
	}

	headers := cloneHeaders(session.headers)
	headers["Accept"] = "application/json"
	headers["OpenAI-Sentinel-Chat-Requirements-Token"] = session.requirementsToken
	if session.proofToken != "" {
		headers["OpenAI-Sentinel-Proof-Token"] = session.proofToken
	}

	for _, fileID := range fileIDs {
		for attempt := 0; attempt < 30; attempt++ {
			req, err := newJSONRequest(
				ctx,
				http.MethodGet,
				"https://chatgpt.com/backend-api/files/"+url.PathEscape(fileID)+"/download",
				headers,
				nil,
			)
			if err != nil {
				return dto.ImageDataItem{}, err
			}

			resp, err := session.client.Do(req)
			if err != nil {
				return dto.ImageDataItem{}, err
			}
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusConflict {
				resp.Body.Close()
				time.Sleep(1 * time.Second)
				continue
			}
			if resp.StatusCode >= http.StatusBadRequest {
				snippet := readResponseSnippet(resp, 512)
				return dto.ImageDataItem{}, badGateway(firstNonEmpty(snippet, resp.Status))
			}

			payload := map[string]any{}
			if err := decodeJSONResponse(resp, &payload); err != nil {
				return dto.ImageDataItem{}, err
			}
			downloadURL := firstNonEmpty(asString(payload["download_url"]), asString(payload["url"]))
			if downloadURL == "" {
				time.Sleep(1 * time.Second)
				continue
			}

			binaryReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
			if err != nil {
				return dto.ImageDataItem{}, err
			}
			binaryResp, err := session.client.Do(binaryReq)
			if err != nil {
				return dto.ImageDataItem{}, err
			}
			if binaryResp.StatusCode >= http.StatusBadRequest {
				snippet := readResponseSnippet(binaryResp, 512)
				return dto.ImageDataItem{}, badGateway(firstNonEmpty(snippet, binaryResp.Status))
			}

			data, err := io.ReadAll(binaryResp.Body)
			binaryResp.Body.Close()
			if err != nil {
				return dto.ImageDataItem{}, err
			}

			return dto.ImageDataItem{
				B64JSON:       base64.StdEncoding.EncodeToString(data),
				RevisedPrompt: "",
			}, nil
		}
	}

	return dto.ImageDataItem{}, badGateway("timed out waiting for image download")
}

func (s *ImageUpstreamService) resolveConversationModel(item *model.Account, requested string) string {
	if requested != "" && requested != "gpt-image-1" && requested != "gpt-image-2" {
		return requested
	}
	if item.DefaultModelSlug != "" {
		return item.DefaultModelSlug
	}
	return "gpt-4o"
}

func parseSSE(resp *http.Response) ([]string, string, error) {
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	fileIDs := []string{}
	revisedPrompt := ""
	seen := map[string]struct{}{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		message := map[string]any{}
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			continue
		}

		for _, fileID := range extractFileIDs(message) {
			if _, ok := seen[fileID]; ok {
				continue
			}
			seen[fileID] = struct{}{}
			fileIDs = append(fileIDs, fileID)
		}

		if text := extractMessageText(message); text != "" {
			revisedPrompt = text
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if len(fileIDs) == 0 {
		return nil, revisedPrompt, badGateway("failed to extract image file id")
	}

	return fileIDs, revisedPrompt, nil
}

func buildConversationMessage(content map[string]any) map[string]any {
	return map[string]any{
		"id":      newUUID(),
		"author":  map[string]any{"role": "user"},
		"content": content,
		"metadata": map[string]any{
			"serialization_metadata": map[string]any{
				"custom_symbol_offsets": []any{},
			},
			"cited_text": "",
		},
	}
}

func buildClientContext() map[string]any {
	return map[string]any{
		"is_dark_mode":      false,
		"time_since_loaded": rand.Float64() * 60,
		"page_height":       923,
		"page_width":        926,
		"pixel_ratio":       1.75,
		"screen_height":     1080,
		"screen_width":      1920,
	}
}

func imageSize(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 1024, 1024
	}
	return cfg.Width, cfg.Height
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func extractFileIDs(value any) []string {
	ids := []string{}
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "file_id" {
				if fileID := asString(child); fileID != "" {
					ids = append(ids, fileID)
				}
			}
			if key == "asset_pointer" {
				if text := asString(child); strings.HasPrefix(text, "file-service://") {
					ids = append(ids, strings.TrimPrefix(text, "file-service://"))
				}
			}
			ids = append(ids, extractFileIDs(child)...)
		}
	case []any:
		for _, child := range current {
			ids = append(ids, extractFileIDs(child)...)
		}
	}
	return ids
}

func extractMessageText(payload map[string]any) string {
	message, ok := payload["message"].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := message["content"].(map[string]any)
	if !ok {
		return ""
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch current := part.(type) {
		case string:
			if strings.TrimSpace(current) != "" {
				texts = append(texts, current)
			}
		case map[string]any:
			if text := asString(current["text"]); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "\n")
}

func NormalizeImageInput(data []byte, providedName string, mimeType string, index int) InputImage {
	name := strings.TrimSpace(providedName)
	if name == "" {
		exts, _ := mime.ExtensionsByType(mimeType)
		ext := ".png"
		if len(exts) > 0 {
			ext = exts[0]
		}
		name = fmt.Sprintf("image-%d%s", index+1, ext)
	} else if filepath.Ext(name) == "" {
		exts, _ := mime.ExtensionsByType(mimeType)
		if len(exts) > 0 {
			name += exts[0]
		}
	}

	return InputImage{
		Data:     data,
		FileName: name,
		MimeType: mimeType,
	}
}
