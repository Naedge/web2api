package dto

type CPAPoolCreateRequest struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	SecretKey string `json:"secret_key"`
}

type CPAPoolUpdateRequest struct {
	Name      *string `json:"name"`
	BaseURL   *string `json:"base_url"`
	SecretKey *string `json:"secret_key"`
}

type CPAImportRequest struct {
	Names []string `json:"names"`
}
