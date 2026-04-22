package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"web2api/internal/dto"
)

type ChatService struct {
	accountService *AccountService
	upstream       *ImageUpstreamService
}

func NewChatService(accountService *AccountService, upstream *ImageUpstreamService) *ChatService {
	return &ChatService{
		accountService: accountService,
		upstream:       upstream,
	}
}

func (s *ChatService) ListModels() map[string]any {
	return map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       "gpt-image-1",
				"object":   "model",
				"created":  0,
				"owned_by": "chatgpt2api",
			},
			{
				"id":       "gpt-image-2",
				"object":   "model",
				"created":  0,
				"owned_by": "chatgpt2api",
			},
		},
	}
}

func (s *ChatService) CreateImageCompletion(
	ctx context.Context,
	prompt string,
	model string,
	n int,
) (*dto.ImageResult, error) {
	if n < 1 || n > 4 {
		return nil, badRequest("n must be between 1 and 4")
	}
	return s.generateWithPool(ctx, prompt, model, n)
}

func (s *ChatService) EditImageCompletion(
	ctx context.Context,
	prompt string,
	model string,
	n int,
	images []InputImage,
) (*dto.ImageResult, error) {
	if len(images) == 0 {
		return nil, badRequest("image is required")
	}
	if n < 1 || n > 4 {
		return nil, badRequest("n must be between 1 and 4")
	}
	return s.editWithPool(ctx, prompt, images, model, n)
}

func (s *ChatService) CreateChatCompletion(ctx context.Context, body map[string]any) (map[string]any, error) {
	if !isImageChatRequest(body) {
		return nil, badRequest("only image generation requests are supported on this endpoint")
	}
	if streamed, _ := body["stream"].(bool); streamed {
		return nil, badRequest("stream is not supported for image generation")
	}

	model := strings.TrimSpace(asString(body["model"]))
	if model == "" {
		model = "gpt-image-1"
	}
	n, err := ParseImageCount(body["n"], 1)
	if err != nil {
		return nil, err
	}

	prompt := extractChatPrompt(body)
	if prompt == "" {
		return nil, badRequest("prompt is required")
	}

	imageData, mimeType, hasImage := extractChatImage(body)
	var result *dto.ImageResult
	if hasImage {
		result, err = s.editWithPool(ctx, prompt, []InputImage{NewInputImage(imageData, "image.png", mimeType, 0)}, model, n)
	} else {
		result, err = s.generateWithPool(ctx, prompt, model, n)
	}
	if err != nil {
		return nil, err
	}

	return buildChatImageCompletion(model, result), nil
}

func (s *ChatService) CreateResponse(ctx context.Context, body map[string]any) (map[string]any, error) {
	if streamed, _ := body["stream"].(bool); streamed {
		return nil, badRequest("stream is not supported")
	}
	if !hasResponseImageGenerationTool(body) {
		return nil, badRequest("only image_generation tool requests are supported on this endpoint")
	}

	prompt := extractResponsePrompt(body["input"])
	if prompt == "" {
		return nil, badRequest("input text is required")
	}

	model := strings.TrimSpace(asString(body["model"]))
	if model == "" {
		model = "gpt-5"
	}

	imageData, mimeType, hasImage := extractResponseImage(body["input"])
	var result *dto.ImageResult
	var err error
	if hasImage {
		result, err = s.editWithPool(ctx, prompt, []InputImage{NewInputImage(imageData, "image.png", mimeType, 0)}, "gpt-image-1", 1)
	} else {
		result, err = s.generateWithPool(ctx, prompt, "gpt-image-1", 1)
	}
	if err != nil {
		return nil, err
	}

	output := make([]map[string]any, 0, len(result.Data))
	for index, item := range result.Data {
		b64JSON := strings.TrimSpace(item.B64JSON)
		if b64JSON == "" {
			continue
		}
		output = append(output, map[string]any{
			"id":             fmt.Sprintf("ig_%d", index+1),
			"type":           "image_generation_call",
			"status":         "completed",
			"result":         b64JSON,
			"revised_prompt": FirstNonEmpty(strings.TrimSpace(item.RevisedPrompt), prompt),
		})
	}
	if len(output) == 0 {
		return nil, badGateway("image generation failed")
	}

	created := result.Created
	return map[string]any{
		"id":                  fmt.Sprintf("resp_%d", created),
		"object":              "response",
		"created_at":          created,
		"status":              "completed",
		"error":               nil,
		"incomplete_details":  nil,
		"model":               model,
		"output":              output,
		"parallel_tool_calls": false,
	}, nil
}

func (s *ChatService) generateWithPool(
	ctx context.Context,
	prompt string,
	model string,
	n int,
) (*dto.ImageResult, error) {
	created := int64(0)
	items := make([]dto.ImageDataItem, 0, n)

	for index := 1; index <= n; index++ {
		for {
			accessToken, err := s.accountService.GetAvailableAccessToken(ctx)
			if err != nil {
				break
			}

			item, err := s.upstream.Generate(ctx, accessToken, prompt, model)
			if err == nil {
				_ = s.accountService.MarkSuccess(ctx, accessToken)
				if created == 0 {
					created = time.Now().Unix()
				}
				items = append(items, item)
				break
			}

			_ = s.accountService.MarkFailure(ctx, accessToken, false)
			if isTokenInvalidError(err.Error()) {
				_ = s.accountService.RemoveToken(ctx, accessToken)
				continue
			}
			break
		}
	}

	if len(items) == 0 {
		return nil, badGateway("image generation failed")
	}

	return &dto.ImageResult{
		Created: created,
		Data:    items,
	}, nil
}

func (s *ChatService) editWithPool(
	ctx context.Context,
	prompt string,
	images []InputImage,
	model string,
	n int,
) (*dto.ImageResult, error) {
	if len(images) == 0 {
		return nil, badRequest("image is required")
	}

	created := int64(0)
	items := make([]dto.ImageDataItem, 0, n)

	for index := 1; index <= n; index++ {
		for {
			accessToken, err := s.accountService.GetAvailableAccessToken(ctx)
			if err != nil {
				break
			}

			item, err := s.upstream.Edit(ctx, accessToken, prompt, model, images)
			if err == nil {
				_ = s.accountService.MarkSuccess(ctx, accessToken)
				if created == 0 {
					created = time.Now().Unix()
				}
				items = append(items, item)
				break
			}

			_ = s.accountService.MarkFailure(ctx, accessToken, false)
			if isTokenInvalidError(err.Error()) {
				_ = s.accountService.RemoveToken(ctx, accessToken)
				continue
			}
			break
		}
	}

	if len(items) == 0 {
		return nil, badGateway("image edit failed")
	}

	return &dto.ImageResult{
		Created: created,
		Data:    items,
	}, nil
}

func isImageChatRequest(body map[string]any) bool {
	model := strings.TrimSpace(asString(body["model"]))
	if model == "gpt-image-1" || model == "gpt-image-2" {
		return true
	}
	modalities, ok := body["modalities"].([]any)
	if !ok {
		return false
	}
	for _, item := range modalities {
		if strings.EqualFold(strings.TrimSpace(asString(item)), "image") {
			return true
		}
	}
	return false
}

func hasResponseImageGenerationTool(body map[string]any) bool {
	if tools, ok := body["tools"].([]any); ok {
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if ok && strings.TrimSpace(asString(tool["type"])) == "image_generation" {
				return true
			}
		}
	}

	if toolChoice, ok := body["tool_choice"].(map[string]any); ok {
		return strings.TrimSpace(asString(toolChoice["type"])) == "image_generation"
	}
	return false
}

func extractResponsePrompt(inputValue any) string {
	switch current := inputValue.(type) {
	case string:
		return strings.TrimSpace(current)
	case map[string]any:
		role := strings.ToLower(strings.TrimSpace(asString(current["role"])))
		if role != "" && role != "user" {
			return ""
		}
		return extractPromptFromMessageContent(current["content"])
	case []any:
		promptParts := make([]string, 0, len(current))
		for _, item := range current {
			rawItem, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if strings.TrimSpace(asString(rawItem["type"])) == "input_text" {
				text := strings.TrimSpace(asString(rawItem["text"]))
				if text != "" {
					promptParts = append(promptParts, text)
				}
				continue
			}
			role := strings.ToLower(strings.TrimSpace(asString(rawItem["role"])))
			if role != "" && role != "user" {
				continue
			}
			prompt := extractPromptFromMessageContent(rawItem["content"])
			if prompt != "" {
				promptParts = append(promptParts, prompt)
			}
		}
		return strings.TrimSpace(strings.Join(promptParts, "\n"))
	default:
		return ""
	}
}

func extractPromptFromMessageContent(content any) string {
	switch current := content.(type) {
	case string:
		return strings.TrimSpace(current)
	case []any:
		parts := make([]string, 0, len(current))
		for _, item := range current {
			rawItem, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemType := strings.TrimSpace(asString(rawItem["type"]))
			if itemType == "text" {
				text := strings.TrimSpace(asString(rawItem["text"]))
				if text != "" {
					parts = append(parts, text)
				}
				continue
			}
			if itemType == "input_text" {
				text := strings.TrimSpace(FirstNonEmpty(asString(rawItem["text"]), asString(rawItem["input_text"])))
				if text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func extractImageFromMessageContent(content any) ([]byte, string, bool) {
	items, ok := content.([]any)
	if !ok {
		return nil, "", false
	}

	for _, item := range items {
		rawItem, ok := item.(map[string]any)
		if !ok {
			continue
		}

		itemType := strings.TrimSpace(asString(rawItem["type"]))
		switch itemType {
		case "image_url":
			urlValue := rawItem["image_url"]
			if imageData, mimeType, ok := decodeOptionalDataURL(urlValue); ok {
				return imageData, mimeType, true
			}
		case "input_image":
			if imageData, mimeType, ok := decodeOptionalDataURL(rawItem["image_url"]); ok {
				return imageData, mimeType, true
			}
		}
	}

	return nil, "", false
}

func extractChatImage(body map[string]any) ([]byte, string, bool) {
	messages, ok := body["messages"].([]any)
	if !ok {
		return nil, "", false
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message, ok := messages[index].(map[string]any)
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(asString(message["role"]))) != "user" {
			continue
		}
		if imageData, mimeType, ok := extractImageFromMessageContent(message["content"]); ok {
			return imageData, mimeType, true
		}
	}
	return nil, "", false
}

func extractChatPrompt(body map[string]any) string {
	directPrompt := strings.TrimSpace(asString(body["prompt"]))
	if directPrompt != "" {
		return directPrompt
	}

	messages, ok := body["messages"].([]any)
	if !ok {
		return ""
	}

	promptParts := make([]string, 0, len(messages))
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(asString(message["role"])))
		if role != "user" {
			continue
		}
		prompt := extractPromptFromMessageContent(message["content"])
		if prompt != "" {
			promptParts = append(promptParts, prompt)
		}
	}
	return strings.TrimSpace(strings.Join(promptParts, "\n"))
}

func extractResponseImage(inputValue any) ([]byte, string, bool) {
	if current, ok := inputValue.(map[string]any); ok {
		return extractImageFromMessageContent(current["content"])
	}

	items, ok := inputValue.([]any)
	if !ok {
		return nil, "", false
	}
	for index := len(items) - 1; index >= 0; index-- {
		item, ok := items[index].(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(asString(item["type"])) == "input_image" {
			if imageData, mimeType, ok := decodeOptionalDataURL(item["image_url"]); ok {
				return imageData, mimeType, true
			}
		}
		if content := item["content"]; content != nil {
			if imageData, mimeType, ok := extractImageFromMessageContent(content); ok {
				return imageData, mimeType, true
			}
		}
	}
	return nil, "", false
}

func decodeOptionalDataURL(value any) ([]byte, string, bool) {
	var urlValue string
	switch current := value.(type) {
	case string:
		urlValue = current
	case map[string]any:
		urlValue = asString(current["url"])
	default:
		return nil, "", false
	}

	if !strings.HasPrefix(urlValue, "data:") {
		return nil, "", false
	}

	header, data, ok := strings.Cut(strings.TrimPrefix(urlValue, "data:"), ",")
	if !ok {
		return nil, "", false
	}

	mimeType := strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
	if mimeType == "" {
		mimeType = "image/png"
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", false
	}
	return decoded, mimeType, true
}

func buildChatImageCompletion(model string, imageResult *dto.ImageResult) map[string]any {
	markdownImages := make([]string, 0, len(imageResult.Data))
	for index, item := range imageResult.Data {
		b64JSON := strings.TrimSpace(item.B64JSON)
		if b64JSON == "" {
			continue
		}
		markdownImages = append(markdownImages, fmt.Sprintf("![image_%d](data:image/png;base64,%s)", index+1, b64JSON))
	}

	textContent := "Image generation completed."
	if len(markdownImages) > 0 {
		textContent = strings.Join(markdownImages, "\n\n")
	}

	return map[string]any{
		"id":      "chatcmpl-" + newHexID(32),
		"object":  "chat.completion",
		"created": imageResult.Created,
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": textContent,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
}

func isTokenInvalidError(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(text, "token_invalidated") ||
		strings.Contains(text, "token_revoked") ||
		strings.Contains(text, "authentication token has been invalidated") ||
		strings.Contains(text, "invalidated oauth token")
}
