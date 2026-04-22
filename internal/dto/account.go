package dto

type AccountCreateRequest struct {
	Tokens []string `json:"tokens"`
}

type AccountDeleteRequest struct {
	Tokens []string `json:"tokens"`
}

type AccountRefreshRequest struct {
	AccessTokens []string `json:"access_tokens"`
}

type AccountUpdateRequest struct {
	AccessToken string  `json:"access_token"`
	Type        *string `json:"type"`
	Status      *string `json:"status"`
	Quota       *int    `json:"quota"`
}

type AccountPublic struct {
	ID               string           `json:"id"`
	AccessToken      string           `json:"access_token"`
	Type             string           `json:"type"`
	Status           string           `json:"status"`
	Quota            int              `json:"quota"`
	Email            string           `json:"email,omitempty"`
	UserID           string           `json:"user_id,omitempty"`
	LimitsProgress   []map[string]any `json:"limits_progress"`
	DefaultModelSlug string           `json:"default_model_slug,omitempty"`
	RestoreAt        string           `json:"restoreAt,omitempty"`
	Success          int              `json:"success"`
	Fail             int              `json:"fail"`
	LastUsedAt       string           `json:"lastUsedAt,omitempty"`
}
