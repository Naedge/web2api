package dto

type ImageGenerationRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
	N      int    `json:"n"`
}

type ImageDataItem struct {
	B64JSON       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}

type ImageResult struct {
	Created int64           `json:"created"`
	Data    []ImageDataItem `json:"data"`
}
