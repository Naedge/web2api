package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
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

const imageProofUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

type ImageUpstreamService struct {
	accountService *AccountService
	proxyService   *ProxyService
	tlsVerify      bool
}

func NewImageUpstreamService(
	accountService *AccountService,
	proxyService *ProxyService,
	tlsVerify bool,
) *ImageUpstreamService {
	return &ImageUpstreamService{
		accountService: accountService,
		proxyService:   proxyService,
		tlsVerify:      tlsVerify,
	}
}

func (s *ImageUpstreamService) Generate(
	ctx context.Context,
	accessToken string,
	prompt string,
	model string,
) (dto.ImageDataItem, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return dto.ImageDataItem{}, badGateway("prompt is required")
	}
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
	result, err := s.sendConversation(
		ctx,
		session,
		conversationModel,
		prompt,
		nil,
		nil,
	)
	if err != nil {
		return dto.ImageDataItem{}, err
	}

	fileIDs := result.FileIDs
	if result.ConversationID != "" && len(fileIDs) == 0 {
		fileIDs = s.pollImageIDs(ctx, session, result.ConversationID)
	}
	if len(fileIDs) == 0 {
		if strings.TrimSpace(result.Text) != "" {
			return dto.ImageDataItem{}, badGateway(result.Text)
		}
		return dto.ImageDataItem{}, badGateway("no image returned from upstream")
	}

	return s.downloadImage(ctx, session, result.ConversationID, fileIDs[0], prompt)
}

func (s *ImageUpstreamService) Edit(
	ctx context.Context,
	accessToken string,
	prompt string,
	model string,
	images []InputImage,
) (dto.ImageDataItem, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return dto.ImageDataItem{}, badGateway("prompt is required")
	}
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
	attachments := make([]map[string]any, 0, len(images))
	inputFileIDs := make(map[string]struct{}, len(images))
	for _, image := range images {
		fileID, width, height, err := s.uploadImage(ctx, session, image)
		if err != nil {
			return dto.ImageDataItem{}, err
		}

		inputFileIDs[fileID] = struct{}{}
		parts = append(parts, map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": "sediment://" + fileID,
			"size_bytes":    len(image.Data),
			"width":         width,
			"height":        height,
		})
		attachments = append(attachments, map[string]any{
			"id":           fileID,
			"size":         len(image.Data),
			"name":         image.FileName,
			"mime_type":    image.MimeType,
			"width":        width,
			"height":       height,
			"source":       "local",
			"is_big_paste": false,
		})
	}

	conversationModel := s.resolveConversationModel(item, model)
	result, err := s.sendConversation(
		ctx,
		session,
		conversationModel,
		prompt,
		parts,
		attachments,
	)
	if err != nil {
		return dto.ImageDataItem{}, err
	}

	fileIDs := filterOutputFileIDs(result.FileIDs, inputFileIDs)
	if result.ConversationID != "" && len(fileIDs) == 0 {
		fileIDs = filterOutputFileIDs(
			s.pollImageIDs(ctx, session, result.ConversationID),
			inputFileIDs,
		)
	}
	if len(fileIDs) == 0 {
		if strings.TrimSpace(result.Text) != "" {
			return dto.ImageDataItem{}, badGateway(result.Text)
		}
		return dto.ImageDataItem{}, badGateway("no image returned from upstream")
	}

	return s.downloadImage(ctx, session, result.ConversationID, fileIDs[0], prompt)
}

type upstreamSession struct {
	client            *http.Client
	headers           map[string]string
	accessToken       string
	deviceID          string
	requirementsToken string
	proofToken        string
}

type upstreamConversationResult struct {
	ConversationID string
	FileIDs        []string
	Text           string
}

func (s *ImageUpstreamService) newSession(ctx context.Context, account any) (*upstreamSession, error) {
	item, ok := account.(*model.Account)
	if !ok {
		return nil, badGateway("invalid account")
	}

	proxyConfig, err := s.proxyService.ActiveConfig(ctx)
	if err != nil {
		return nil, err
	}

	client, err := newHTTPClient(s.tlsVerify, 120*time.Second, proxyConfig)
	if err != nil {
		return nil, err
	}

	pageHeaders := map[string]string{
		"Sec-CH-UA":  firstNonEmpty(item.SecCHUA, `"Microsoft Edge";v="131", "Chromium";v="131", "Not_A Brand";v="24"`),
		"User-Agent": imageProofUserAgent,
	}
	pageReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/", nil)
	if err != nil {
		return nil, err
	}
	for key, value := range pageHeaders {
		pageReq.Header.Set(key, value)
	}
	pageResp, err := client.Do(pageReq)
	if err == nil {
		body, _ := io.ReadAll(pageResp.Body)
		pageResp.Body.Close()
		updateProofBuildInfoFromHTML(string(body))
	}

	deviceID := strings.TrimSpace(item.OAIDeviceID)
	if client.Jar != nil {
		if parsedURL, parseErr := url.Parse("https://chatgpt.com/"); parseErr == nil {
			for _, cookie := range client.Jar.Cookies(parsedURL) {
				if strings.EqualFold(cookie.Name, "oai-did") && strings.TrimSpace(cookie.Value) != "" {
					deviceID = strings.TrimSpace(cookie.Value)
					break
				}
			}
		}
	}
	if deviceID == "" {
		deviceID = newUUID()
	}

	headers := s.buildImageHeaders(item, deviceID)

	requirementsToken, proofToken, err := s.fetchRequirements(ctx, client, headers, item.AccessToken, deviceID)
	if err != nil {
		return nil, err
	}

	return &upstreamSession{
		client:            client,
		headers:           headers,
		accessToken:       item.AccessToken,
		deviceID:          deviceID,
		requirementsToken: requirementsToken,
		proofToken:        proofToken,
	}, nil
}

func (s *ImageUpstreamService) fetchRequirements(
	ctx context.Context,
	client *http.Client,
	headers map[string]string,
	accessToken string,
	deviceID string,
) (string, string, error) {
	reqHeaders := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
		"Oai-Device-Id": deviceID,
		"User-Agent":    imageProofUserAgent,
	}

	req, err := newJSONRequest(
		ctx,
		http.MethodPost,
		"https://chatgpt.com/backend-api/sentinel/chat-requirements",
		reqHeaders,
		map[string]any{
			"p": getRequirementsToken(imageProofUserAgent),
		},
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
				imageProofUserAgent,
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
	attachments []map[string]any,
) (*upstreamConversationResult, error) {
	if attachments == nil {
		attachments = []map[string]any{}
	}
	message := map[string]any{
		"id":     newUUID(),
		"author": map[string]any{"role": "user"},
		"metadata": map[string]any{
			"attachments": attachments,
		},
	}
	if len(multimodalParts) > 0 {
		parts := make([]any, 0, len(multimodalParts)+1)
		for _, part := range multimodalParts {
			parts = append(parts, part)
		}
		parts = append(parts, prompt)
		message["content"] = map[string]any{
			"content_type": "multimodal_text",
			"parts":        parts,
		}
	} else {
		message["content"] = map[string]any{
			"content_type": "text",
			"parts":        []any{prompt},
		}
	}

	body := map[string]any{
		"action":                               "next",
		"messages":                             []any{message},
		"model":                                model,
		"parent_message_id":                    newUUID(),
		"history_and_training_disabled":        false,
		"timezone_offset_min":                  -480,
		"timezone":                             "America/Los_Angeles",
		"conversation_mode":                    map[string]any{"kind": "primary_assistant"},
		"conversation_origin":                  nil,
		"force_paragen":                        false,
		"force_paragen_model_slug":             "",
		"force_rate_limit":                     false,
		"force_use_sse":                        true,
		"paragen_cot_summary_display_override": "allow",
		"reset_rate_limits":                    false,
		"suggestions":                          []any{},
		"supported_encodings":                  []any{},
		"system_hints":                         []string{"picture_v2"},
		"variant_purpose":                      "comparison_implicit",
		"websocket_request_id":                 newUUID(),
		"client_contextual_info":               buildClientContext(),
	}
	if len(multimodalParts) == 0 {
		body["conversation_origin"] = nil
		body["paragen_stream_type_override"] = nil
	}

	headers := cloneHeaders(session.headers)
	headers["Authorization"] = "Bearer " + session.accessToken
	headers["Accept"] = "text/event-stream"
	headers["Accept-Language"] = "zh-CN,zh;q=0.9,en;q=0.8"
	headers["Content-Type"] = "application/json"
	headers["Oai-Device-Id"] = session.deviceID
	headers["Oai-Language"] = "zh-CN"
	headers["Oai-Client-Build-Number"] = "5955942"
	headers["Oai-Client-Version"] = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
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
		return nil, err
	}

	resp, err := session.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		snippet := readResponseSnippet(resp, 1024)
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, unauthorized(firstNonEmpty(snippet, "invalid access token"))
		}
		return nil, badGateway(firstNonEmpty(snippet, resp.Status))
	}

	return parseBrowserSSE(resp)
}

func (s *ImageUpstreamService) uploadImage(
	ctx context.Context,
	session *upstreamSession,
	image InputImage,
) (string, int, int, error) {
	width, height := imageSize(image.Data)
	body := map[string]any{
		"file_name":           image.FileName,
		"file_size":           len(image.Data),
		"use_case":            "multimodal",
		"timezone_offset_min": -480,
		"reset_rate_limits":   false,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + session.accessToken,
		"Content-Type":  "application/json",
		"Oai-Device-Id": session.deviceID,
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

	putHeaders := map[string]string{
		"Content-Type":   image.MimeType,
		"X-Ms-Blob-Type": "BlockBlob",
		"X-Ms-Version":   "2020-04-08",
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(image.Data))
	if err != nil {
		return "", 0, 0, err
	}
	for key, value := range putHeaders {
		putReq.Header.Set(key, value)
	}
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
		"https://chatgpt.com/backend-api/files/process_upload_stream",
		headers,
		map[string]any{
			"file_id":             fileID,
			"use_case":            "multimodal",
			"index_for_retrieval": false,
			"file_name":           image.FileName,
		},
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
	conversationID string,
	fileID string,
	revisedPrompt string,
) (dto.ImageDataItem, error) {
	downloadURL, err := s.fetchDownloadURL(ctx, session, conversationID, fileID)
	if err != nil {
		return dto.ImageDataItem{}, err
	}
	if downloadURL == "" {
		return dto.ImageDataItem{}, badGateway("failed to get download url")
	}

	binaryReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return dto.ImageDataItem{}, err
	}
	for key, value := range session.headers {
		if strings.TrimSpace(value) != "" {
			binaryReq.Header.Set(key, value)
		}
	}
	binaryReq.Header.Set("Accept", "*/*")
	binaryResp, err := session.client.Do(binaryReq)
	if err != nil {
		return dto.ImageDataItem{}, err
	}
	defer binaryResp.Body.Close()

	data, err := io.ReadAll(binaryResp.Body)
	if err != nil {
		return dto.ImageDataItem{}, err
	}
	if binaryResp.StatusCode >= http.StatusBadRequest || len(data) == 0 {
		return dto.ImageDataItem{}, badGateway("download image failed")
	}

	return dto.ImageDataItem{
		B64JSON:       base64.StdEncoding.EncodeToString(data),
		RevisedPrompt: revisedPrompt,
	}, nil
}

func (s *ImageUpstreamService) resolveConversationModel(item *model.Account, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "gpt-image-1"
	}
	switch requested {
	case "gpt-image-1":
		return "auto"
	case "gpt-image-2":
		if strings.TrimSpace(item.Type) == "Free" {
			return "auto"
		}
		return "gpt-5-3"
	default:
		return requested
	}
}

func parseBrowserSSE(resp *http.Response) (*upstreamConversationResult, error) {
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	result := &upstreamConversationResult{
		FileIDs: []string{},
	}
	seen := map[string]struct{}{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			break
		}

		collectPrefixedIDs(payload, "file-service://", "", seen, &result.FileIDs)
		collectPrefixedIDs(payload, "sediment://", "sed:", seen, &result.FileIDs)

		message := map[string]any{}
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			continue
		}

		if conversationID := strings.TrimSpace(asString(message["conversation_id"])); conversationID != "" {
			result.ConversationID = conversationID
		}
		if messageType := strings.TrimSpace(asString(message["type"])); messageType == "resume_conversation_token" ||
			messageType == "message_marker" || messageType == "message_stream_complete" {
			if conversationID := strings.TrimSpace(asString(message["conversation_id"])); conversationID != "" {
				result.ConversationID = conversationID
			}
		}
		if value, ok := message["v"].(map[string]any); ok {
			if conversationID := strings.TrimSpace(asString(value["conversation_id"])); conversationID != "" {
				result.ConversationID = conversationID
			}
		}

		if messageBody, ok := message["message"].(map[string]any); ok {
			if content, ok := messageBody["content"].(map[string]any); ok {
				if asString(content["content_type"]) == "text" {
					if parts, ok := content["parts"].([]any); ok && len(parts) > 0 {
						result.Text += asString(parts[0])
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *ImageUpstreamService) fetchDownloadURL(
	ctx context.Context,
	session *upstreamSession,
	conversationID string,
	fileID string,
) (string, error) {
	rawID := fileID
	endpoint := ""
	if strings.HasPrefix(fileID, "sed:") {
		rawID = strings.TrimPrefix(fileID, "sed:")
		endpoint = "https://chatgpt.com/backend-api/conversation/" + url.PathEscape(conversationID) + "/attachment/" + url.PathEscape(rawID) + "/download"
	} else {
		endpoint = "https://chatgpt.com/backend-api/files/" + url.PathEscape(rawID) + "/download"
	}

	headers := map[string]string{
		"Authorization": "Bearer " + session.accessToken,
		"Oai-Device-Id": session.deviceID,
		"Accept":        "*/*",
		"User-Agent":    session.headers["User-Agent"],
	}
	req, err := newJSONRequest(ctx, http.MethodGet, endpoint, headers, nil)
	if err != nil {
		return "", err
	}

	resp, err := session.client.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		return "", nil
	}

	payload := map[string]any{}
	if err := decodeJSONResponse(resp, &payload); err != nil {
		return "", err
	}

	downloadURL := firstNonEmpty(asString(payload["download_url"]), asString(payload["url"]))
	return downloadURL, nil
}

func (s *ImageUpstreamService) pollImageIDs(
	ctx context.Context,
	session *upstreamSession,
	conversationID string,
) []string {
	started := time.Now()
	for time.Since(started) < 180*time.Second {
		headers := map[string]string{
			"Authorization": "Bearer " + session.accessToken,
			"Oai-Device-Id": session.deviceID,
			"Accept":        "*/*",
			"User-Agent":    session.headers["User-Agent"],
		}
		req, err := newJSONRequest(
			ctx,
			http.MethodGet,
			"https://chatgpt.com/backend-api/conversation/"+url.PathEscape(conversationID),
			headers,
			nil,
		)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		resp, err := session.client.Do(req)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			time.Sleep(3 * time.Second)
			continue
		}

		payload := map[string]any{}
		if err := decodeJSONResponse(resp, &payload); err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		fileIDs := extractImageIDsFromMapping(payload["mapping"])
		if len(fileIDs) > 0 {
			return fileIDs
		}
		time.Sleep(3 * time.Second)
	}
	return []string{}
}

func (s *ImageUpstreamService) buildImageHeaders(item *model.Account, deviceID string) map[string]string {
	return map[string]string{
		"User-Agent":         imageProofUserAgent,
		"Accept":             "*/*",
		"Accept-Language":    "en-US,en;q=0.9",
		"Origin":             "https://chatgpt.com",
		"Referer":            "https://chatgpt.com/",
		"Sec-CH-UA":          firstNonEmpty(item.SecCHUA, `"Microsoft Edge";v="131", "Chromium";v="131", "Not_A Brand";v="24"`),
		"Sec-CH-UA-Mobile":   firstNonEmpty(item.SecCHUAMobile, "?0"),
		"Sec-CH-UA-Platform": firstNonEmpty(item.SecCHUAPlatform, `"Windows"`),
		"Sec-Fetch-Dest":     "empty",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Site":     "same-origin",
		"Oai-Device-Id":      deviceID,
	}
}

func collectPrefixedIDs(
	payload string,
	prefix string,
	storedPrefix string,
	seen map[string]struct{},
	fileIDs *[]string,
) {
	start := 0
	for {
		index := strings.Index(payload[start:], prefix)
		if index < 0 {
			return
		}
		index += start + len(prefix)
		tail := payload[index:]
		builder := strings.Builder{}
		for _, char := range tail {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
				builder.WriteRune(char)
				continue
			}
			break
		}
		if builder.Len() > 0 {
			value := storedPrefix + builder.String()
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				*fileIDs = append(*fileIDs, value)
			}
		}
		start = index
	}
}

func extractImageIDsFromMapping(value any) []string {
	mapping, ok := value.(map[string]any)
	if !ok {
		return []string{}
	}

	fileIDs := []string{}
	seen := map[string]struct{}{}
	for _, rawNode := range mapping {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		message, ok := node["message"].(map[string]any)
		if !ok {
			continue
		}
		author, _ := message["author"].(map[string]any)
		metadata, _ := message["metadata"].(map[string]any)
		content, _ := message["content"].(map[string]any)
		if asString(author["role"]) != "tool" {
			continue
		}
		if asString(metadata["async_task_type"]) != "image_gen" {
			continue
		}
		if asString(content["content_type"]) != "multimodal_text" {
			continue
		}

		parts, _ := content["parts"].([]any)
		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			pointer := asString(part["asset_pointer"])
			switch {
			case strings.HasPrefix(pointer, "file-service://"):
				value := strings.TrimPrefix(pointer, "file-service://")
				if _, ok := seen[value]; !ok {
					seen[value] = struct{}{}
					fileIDs = append(fileIDs, value)
				}
			case strings.HasPrefix(pointer, "sediment://"):
				value := "sed:" + strings.TrimPrefix(pointer, "sediment://")
				if _, ok := seen[value]; !ok {
					seen[value] = struct{}{}
					fileIDs = append(fileIDs, value)
				}
			}
		}
	}

	return fileIDs
}

func filterOutputFileIDs(fileIDs []string, inputFileIDs map[string]struct{}) []string {
	filtered := make([]string, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if _, ok := inputFileIDs[canonicalFileID(fileID)]; ok {
			continue
		}
		filtered = append(filtered, fileID)
	}
	return filtered
}

func canonicalFileID(fileID string) string {
	if strings.HasPrefix(fileID, "sed:") {
		return strings.TrimPrefix(fileID, "sed:")
	}
	return fileID
}

func buildClientContext() map[string]any {
	return map[string]any{
		"is_dark_mode":      false,
		"time_since_loaded": rand.Intn(451) + 50,
		"page_height":       rand.Intn(501) + 500,
		"page_width":        rand.Intn(1001) + 1000,
		"pixel_ratio":       1.2,
		"screen_height":     rand.Intn(401) + 800,
		"screen_width":      rand.Intn(1001) + 1200,
	}
}

func imageSize(data []byte) (int, int) {
	if len(data) >= 24 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		width := binary.BigEndian.Uint32(data[16:20])
		height := binary.BigEndian.Uint32(data[20:24])
		return int(width), int(height)
	}

	if len(data) >= 4 && data[0] == 0xff && data[1] == 0xd8 {
		offset := 2
		for offset+9 < len(data) {
			if data[offset] != 0xff {
				break
			}
			marker := data[offset+1]
			offset += 2
			if marker == 0xc0 || marker == 0xc1 || marker == 0xc2 {
				if offset+5 >= len(data) {
					break
				}
				height := binary.BigEndian.Uint16(data[offset+1 : offset+3])
				width := binary.BigEndian.Uint16(data[offset+3 : offset+5])
				return int(width), int(height)
			}
			if offset+1 >= len(data) {
				break
			}
			segmentLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			if segmentLength < 2 {
				break
			}
			offset += segmentLength
		}
	}

	return 1024, 1024
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

func NewInputImage(data []byte, fileName string, mimeType string, _ int) InputImage {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "image.png"
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return InputImage{
		Data:     data,
		FileName: fileName,
		MimeType: mimeType,
	}
}
