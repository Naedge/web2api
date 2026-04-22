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
	now := time.Now().Unix()
	return map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       "gpt-image-1",
				"object":   "model",
				"created":  now,
				"owned_by": "web2api",
			},
			{
				"id":       "gpt-image-2",
				"object":   "model",
				"created":  now,
				"owned_by": "web2api",
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
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, badRequest("prompt is required")
	}
	if model == "" {
		model = "gpt-image-1"
	}
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		return nil, badRequest("n must be between 1 and 4")
	}

	result := &dto.ImageResult{
		Created: time.Now().Unix(),
		Data:    make([]dto.ImageDataItem, 0, n),
	}
	for i := 0; i < n; i++ {
		item, err := s.generateSingle(ctx, prompt, model, nil)
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, item)
	}

	return result, nil
}

func (s *ChatService) EditImageCompletion(
	ctx context.Context,
	prompt string,
	model string,
	n int,
	images []InputImage,
) (*dto.ImageResult, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, badRequest("prompt is required")
	}
	if len(images) == 0 {
		return nil, badRequest("image is required")
	}
	if model == "" {
		model = "gpt-image-1"
	}
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		return nil, badRequest("n must be between 1 and 4")
	}

	result := &dto.ImageResult{
		Created: time.Now().Unix(),
		Data:    make([]dto.ImageDataItem, 0, n),
	}
	for i := 0; i < n; i++ {
		item, err := s.generateSingle(ctx, prompt, model, images)
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, item)
	}

	return result, nil
}

func (s *ChatService) CreateChatCompletion(ctx context.Context, body map[string]any) (map[string]any, error) {
	prompt, images, model, err := parseChatPayload(body)
	if err != nil {
		return nil, err
	}

	var result *dto.ImageResult
	if len(images) == 0 {
		result, err = s.CreateImageCompletion(ctx, prompt, model, 1)
	} else {
		result, err = s.EditImageCompletion(ctx, prompt, model, 1, images)
	}
	if err != nil {
		return nil, err
	}

	content := []map[string]any{
		{
			"type": "text",
			"text": firstNonEmpty(result.Data[0].RevisedPrompt, prompt),
		},
	}
	for _, item := range result.Data {
		content = append(content, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:image/png;base64," + item.B64JSON,
			},
		})
	}

	return map[string]any{
		"id":      "chatcmpl_" + newHexID(24),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   firstNonEmpty(model, "gpt-image-1"),
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}, nil
}

func (s *ChatService) CreateResponse(ctx context.Context, body map[string]any) (map[string]any, error) {
	prompt, images, model, err := parseResponsePayload(body)
	if err != nil {
		return nil, err
	}

	var result *dto.ImageResult
	if len(images) == 0 {
		result, err = s.CreateImageCompletion(ctx, prompt, model, 1)
	} else {
		result, err = s.EditImageCompletion(ctx, prompt, model, 1, images)
	}
	if err != nil {
		return nil, err
	}

	output := make([]map[string]any, 0, len(result.Data)+1)
	output = append(output, map[string]any{
		"id":     "msg_" + newHexID(24),
		"type":   "message",
		"role":   "assistant",
		"status": "completed",
		"content": []map[string]any{
			{
				"type": "output_text",
				"text": firstNonEmpty(result.Data[0].RevisedPrompt, prompt),
			},
		},
	})
	for _, item := range result.Data {
		output = append(output, map[string]any{
			"id":     "igc_" + newHexID(24),
			"type":   "image_generation_call",
			"status": "completed",
			"result": item.B64JSON,
		})
	}

	return map[string]any{
		"id":         "resp_" + newHexID(24),
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      firstNonEmpty(model, "gpt-image-1"),
		"output":     output,
	}, nil
}

func (s *ChatService) generateSingle(
	ctx context.Context,
	prompt string,
	model string,
	images []InputImage,
) (dto.ImageDataItem, error) {
	for attempt := 0; attempt < 5; attempt++ {
		accessToken, err := s.accountService.GetAvailableAccessToken(ctx)
		if err != nil {
			return dto.ImageDataItem{}, err
		}

		var item dto.ImageDataItem
		if len(images) == 0 {
			item, err = s.upstream.Generate(ctx, accessToken, prompt, model)
		} else {
			item, err = s.upstream.Edit(ctx, accessToken, prompt, model, images)
		}
		if err == nil {
			_ = s.accountService.MarkSuccess(ctx, accessToken)
			return item, nil
		}

		invalid := false
		if statusErr, ok := err.(*StatusError); ok && statusErr.Code == 401 {
			invalid = true
		}
		_ = s.accountService.MarkFailure(ctx, accessToken, invalid)
	}

	return dto.ImageDataItem{}, badGateway("failed to generate image")
}

func parseChatPayload(body map[string]any) (string, []InputImage, string, error) {
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) == 0 {
		return "", nil, "", badRequest("messages is required")
	}

	model := asString(body["model"])
	for i := len(messages) - 1; i >= 0; i-- {
		message, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		if role := asString(message["role"]); role != "" && role != "user" {
			continue
		}

		prompt, images, err := parseMessageContent(message["content"])
		return prompt, images, model, err
	}

	return "", nil, model, badRequest("user message is required")
}

func parseResponsePayload(body map[string]any) (string, []InputImage, string, error) {
	model := asString(body["model"])
	prompt, images, err := parseMessageContent(body["input"])
	return prompt, images, model, err
}

func parseMessageContent(value any) (string, []InputImage, error) {
	switch current := value.(type) {
	case string:
		current = strings.TrimSpace(current)
		if current == "" {
			return "", nil, badRequest("prompt is required")
		}
		return current, nil, nil
	case []any:
		texts := []string{}
		images := []InputImage{}
		for index, item := range current {
			switch typed := item.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					texts = append(texts, typed)
				}
			case map[string]any:
				itemType := asString(typed["type"])
				switch itemType {
				case "text", "input_text", "output_text":
					text := firstNonEmpty(asString(typed["text"]), asString(typed["content"]))
					if strings.TrimSpace(text) != "" {
						texts = append(texts, text)
					}
				case "image_url", "input_image", "image":
					image, err := decodeImageInput(typed, index)
					if err != nil {
						return "", nil, err
					}
					images = append(images, image)
				default:
					if nestedPrompt, nestedImages, err := parseNestedMessageObject(typed, index); err == nil {
						if nestedPrompt != "" {
							texts = append(texts, nestedPrompt)
						}
						images = append(images, nestedImages...)
					}
				}
			}
		}

		prompt := strings.TrimSpace(strings.Join(texts, "\n"))
		if prompt == "" {
			return "", nil, badRequest("prompt is required")
		}
		return prompt, images, nil
	case map[string]any:
		if content, ok := current["content"]; ok {
			return parseMessageContent(content)
		}
		return parseNestedMessageObject(current, 0)
	default:
		return "", nil, badRequest("invalid prompt content")
	}
}

func parseNestedMessageObject(item map[string]any, index int) (string, []InputImage, error) {
	if content, ok := item["content"]; ok {
		return parseMessageContent(content)
	}

	text := firstNonEmpty(asString(item["text"]), asString(item["prompt"]))
	images := []InputImage{}
	if urlValue := item["image_url"]; urlValue != nil {
		image, err := decodeImageURLValue(urlValue, index)
		if err != nil {
			return "", nil, err
		}
		images = append(images, image)
	}
	if urlValue := item["input_image"]; urlValue != nil {
		image, err := decodeImageURLValue(urlValue, index)
		if err != nil {
			return "", nil, err
		}
		images = append(images, image)
	}

	if text == "" && len(images) == 0 {
		return "", nil, badRequest("invalid prompt content")
	}
	return text, images, nil
}

func decodeImageInput(item map[string]any, index int) (InputImage, error) {
	if value, ok := item["image_url"]; ok {
		return decodeImageURLValue(value, index)
	}
	if value, ok := item["input_image"]; ok {
		return decodeImageURLValue(value, index)
	}
	return InputImage{}, badRequest("invalid image input")
}

func decodeImageURLValue(value any, index int) (InputImage, error) {
	switch current := value.(type) {
	case string:
		return decodeDataURL(current, index)
	case map[string]any:
		return decodeDataURL(asString(current["url"]), index)
	default:
		return InputImage{}, badRequest("image_url must be a data url")
	}
}

func decodeDataURL(value string, index int) (InputImage, error) {
	if !strings.HasPrefix(value, "data:") {
		return InputImage{}, badRequest("only data url image input is supported")
	}

	meta, encoded, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !ok {
		return InputImage{}, badRequest("invalid data url")
	}
	if !strings.HasSuffix(meta, ";base64") {
		return InputImage{}, badRequest("data url must be base64 encoded")
	}

	mimeType := strings.TrimSuffix(meta, ";base64")
	if mimeType == "" {
		mimeType = "image/png"
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return InputImage{}, badRequest(fmt.Sprintf("invalid image base64: %v", err))
	}

	return NormalizeImageInput(data, "", mimeType, index), nil
}
