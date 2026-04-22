package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"web2api/internal/dto"
	"web2api/internal/model"
	"web2api/internal/repository"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"

type AccountService struct {
	repo      *repository.AccountRepository
	tlsVerify bool

	nextMu    sync.Mutex
	nextIndex int
}

func NewAccountService(repo *repository.AccountRepository, tlsVerify bool) *AccountService {
	return &AccountService{
		repo:      repo,
		tlsVerify: tlsVerify,
	}
}

type AccountRefreshError struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

type AccountRefreshResult struct {
	Refreshed int                   `json:"refreshed"`
	Errors    []AccountRefreshError `json:"errors"`
	Items     []dto.AccountPublic   `json:"items"`
}

func (s *AccountService) List(ctx context.Context) ([]dto.AccountPublic, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.toPublicItems(items), nil
}

func (s *AccountService) AddTokens(ctx context.Context, tokens []string) ([]dto.AccountPublic, int, int, error) {
	tokens = normalizeStringList(tokens)
	if len(tokens) == 0 {
		return nil, 0, 0, badRequest("tokens is required")
	}

	added := 0
	skipped := 0
	for _, token := range tokens {
		existing, err := s.repo.GetByToken(ctx, token)
		if err != nil {
			return nil, 0, 0, err
		}
		if existing != nil {
			skipped++
			continue
		}

		item := &model.Account{
			AccessToken:      token,
			Type:             "Free",
			Status:           model.AccountStatusNormal,
			Quota:            0,
			LimitsProgress:   "[]",
			DefaultModelSlug: "gpt-4o",
			UserAgent:        defaultUserAgent,
		}
		if err := s.repo.Create(ctx, item); err != nil {
			return nil, 0, 0, err
		}
		added++
	}

	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	return s.toPublicItems(items), added, skipped, nil
}

func (s *AccountService) DeleteTokens(ctx context.Context, tokens []string) (int64, []dto.AccountPublic, error) {
	tokens = normalizeStringList(tokens)
	if len(tokens) == 0 {
		return 0, nil, badRequest("tokens is required")
	}
	rows, err := s.repo.DeleteByTokens(ctx, tokens)
	if err != nil {
		return 0, nil, err
	}
	items, err := s.List(ctx)
	if err != nil {
		return 0, nil, err
	}
	return rows, items, nil
}

func (s *AccountService) Update(ctx context.Context, req dto.AccountUpdateRequest) (*dto.AccountPublic, error) {
	req.AccessToken = strings.TrimSpace(req.AccessToken)
	if req.AccessToken == "" {
		return nil, badRequest("access_token is required")
	}

	values := map[string]any{}
	if req.Type != nil {
		values["type"] = strings.TrimSpace(*req.Type)
	}
	if req.Status != nil {
		values["status"] = strings.TrimSpace(*req.Status)
	}
	if req.Quota != nil {
		values["quota"] = *req.Quota
	}
	if len(values) == 0 {
		return nil, badRequest("no fields to update")
	}

	if err := s.repo.UpdateByToken(ctx, req.AccessToken, values); err != nil {
		return nil, err
	}

	item, err := s.repo.GetByToken(ctx, req.AccessToken)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, notFound("account not found")
	}

	public := s.toPublicItems([]model.Account{*item})
	return &public[0], nil
}

func (s *AccountService) RefreshAccounts(ctx context.Context, accessTokens []string) ([]dto.AccountPublic, error) {
	result, err := s.RefreshAccountsDetailed(ctx, accessTokens)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (s *AccountService) RefreshAccountsDetailed(
	ctx context.Context,
	accessTokens []string,
) (*AccountRefreshResult, error) {
	accessTokens = normalizeStringList(accessTokens)
	if len(accessTokens) == 0 {
		items, err := s.repo.List(ctx)
		if err != nil {
			return nil, err
		}
		accessTokens = make([]string, 0, len(items))
		for _, item := range items {
			accessTokens = append(accessTokens, item.AccessToken)
		}
	}

	refreshed := 0
	errors := make([]AccountRefreshError, 0)
	for _, token := range accessTokens {
		if err := s.RefreshAccountState(ctx, token); err != nil {
			message := err.Error()
			if statusErr, ok := err.(*StatusError); ok {
				message = statusErr.Message
			}
			errors = append(errors, AccountRefreshError{
				AccessToken: token,
				Error:       message,
			})
			continue
		}
		refreshed++
	}

	items, err := s.repo.ListByTokens(ctx, accessTokens)
	if err != nil {
		return nil, err
	}

	return &AccountRefreshResult{
		Refreshed: refreshed,
		Errors:    errors,
		Items:     s.toPublicItems(items),
	}, nil
}

func (s *AccountService) RefreshLimitedAccounts(ctx context.Context) error {
	items, err := s.repo.ListLimited(ctx)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := s.RefreshAccountState(ctx, item.AccessToken); err != nil {
			continue
		}
	}
	return nil
}

func (s *AccountService) RefreshAccountState(ctx context.Context, accessToken string) error {
	item, err := s.repo.GetByToken(ctx, accessToken)
	if err != nil {
		return err
	}
	if item == nil {
		return notFound("account not found")
	}

	timeoutCtx, cancel := withTimeout(ctx, 25*time.Second)
	defer cancel()

	client := newHTTPClient(s.tlsVerify, 25*time.Second)
	headers := s.accountHeaders(item)

	meReq, err := newJSONRequest(timeoutCtx, http.MethodGet, "https://chatgpt.com/backend-api/me", headers, nil)
	if err != nil {
		return err
	}
	meResp, err := client.Do(meReq)
	if err != nil {
		return err
	}
	if meResp.StatusCode == http.StatusUnauthorized {
		item.Status = model.AccountStatusInvalid
		item.Quota = 0
		if err := s.repo.Save(ctx, item); err != nil {
			return err
		}
		return unauthorized("检测到封号")
	}
	if meResp.StatusCode >= http.StatusBadRequest {
		_ = readResponseSnippet(meResp, 512)
		return fmt.Errorf("me request failed: %s", meResp.Status)
	}

	mePayload := map[string]any{}
	if err := decodeJSONResponse(meResp, &mePayload); err != nil {
		return err
	}

	initReq, err := newJSONRequest(
		timeoutCtx,
		http.MethodPost,
		"https://chatgpt.com/backend-api/conversation/init",
		headers,
		map[string]any{},
	)
	if err != nil {
		return err
	}
	initResp, err := client.Do(initReq)
	if err != nil {
		return err
	}
	if initResp.StatusCode >= http.StatusBadRequest {
		_ = readResponseSnippet(initResp, 512)
		return fmt.Errorf("init request failed: %s", initResp.Status)
	}

	initPayload := map[string]any{}
	if err := decodeJSONResponse(initResp, &initPayload); err != nil {
		return err
	}

	item.Email = asString(mePayload["email"])
	item.UserID = asString(mePayload["id"])

	accountType := searchAccountType(mePayload)
	if accountType == "" {
		accountType = searchAccountType(initPayload)
	}
	if accountType == "" {
		accountType = searchAccountType(decodeJWTBody(accessToken))
	}
	if accountType == "" {
		accountType = item.Type
	}
	item.Type = normalizeAccountType(accountType)

	if limits, ok := initPayload["limits_progress"].([]any); ok {
		list := make([]map[string]any, 0, len(limits))
		for _, entry := range limits {
			if mapped, ok := entry.(map[string]any); ok {
				list = append(list, mapped)
			}
		}
		item.LimitsProgress = encodeJSON(list)
		item.Quota, item.RestoreAt = extractQuotaAndRestoreAt(list)
	}

	if defaultSlug := asString(initPayload["default_model_slug"]); defaultSlug != "" {
		item.DefaultModelSlug = defaultSlug
	}
	if item.Quota > 0 {
		item.Status = model.AccountStatusNormal
	} else {
		item.Status = model.AccountStatusLimited
	}

	return s.repo.Save(ctx, item)
}

func (s *AccountService) GetAvailableAccessToken(ctx context.Context) (string, error) {
	items, err := s.repo.ListAvailableCandidates(ctx)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", badRequest("no available access token")
	}

	s.nextMu.Lock()
	start := s.nextIndex
	s.nextIndex = (s.nextIndex + 1) % len(items)
	s.nextMu.Unlock()

	for offset := 0; offset < len(items); offset++ {
		index := (start + offset) % len(items)
		item := items[index]
		if err := s.RefreshAccountState(ctx, item.AccessToken); err != nil {
			continue
		}

		refreshed, err := s.repo.GetByToken(ctx, item.AccessToken)
		if err != nil {
			return "", err
		}
		if refreshed != nil && refreshed.Status == model.AccountStatusNormal && refreshed.Quota > 0 {
			return refreshed.AccessToken, nil
		}
	}

	return "", badRequest("no available access token")
}

func (s *AccountService) GetByToken(ctx context.Context, accessToken string) (*model.Account, error) {
	return s.repo.GetByToken(ctx, accessToken)
}

func (s *AccountService) MarkSuccess(ctx context.Context, accessToken string) error {
	item, err := s.repo.GetByToken(ctx, accessToken)
	if err != nil {
		return err
	}
	if item == nil {
		return nil
	}

	now := time.Now()
	item.Success++
	item.LastUsedAt = &now
	if item.Quota > 0 {
		item.Quota--
	}
	if item.Quota <= 0 {
		item.Quota = 0
		item.Status = model.AccountStatusLimited
	}
	return s.repo.Save(ctx, item)
}

func (s *AccountService) MarkFailure(ctx context.Context, accessToken string, invalid bool) error {
	item, err := s.repo.GetByToken(ctx, accessToken)
	if err != nil {
		return err
	}
	if item == nil {
		return nil
	}

	now := time.Now()
	item.Fail++
	item.LastUsedAt = &now
	if invalid {
		item.Status = model.AccountStatusInvalid
		item.Quota = 0
	}
	return s.repo.Save(ctx, item)
}

func (s *AccountService) accountHeaders(item *model.Account) map[string]string {
	headers := map[string]string{
		"Accept":             "application/json, text/plain, */*",
		"Authorization":      "Bearer " + item.AccessToken,
		"Content-Type":       "application/json",
		"Origin":             "https://chatgpt.com",
		"Referer":            "https://chatgpt.com/",
		"User-Agent":         firstNonEmpty(item.UserAgent, defaultUserAgent),
		"oai-device-id":      firstNonEmpty(item.OAIDeviceID, newUUID()),
		"oai-language":       "zh-CN",
		"Sec-CH-UA":          firstNonEmpty(item.SecCHUA, `"Chromium";v="135", "Google Chrome";v="135", "Not/A)Brand";v="8"`),
		"Sec-CH-UA-Mobile":   firstNonEmpty(item.SecCHUAMobile, "?0"),
		"Sec-CH-UA-Platform": firstNonEmpty(item.SecCHUAPlatform, `"Windows"`),
	}
	if item.OAISessionID != "" {
		headers["oai-session-id"] = item.OAISessionID
	}
	if item.Impersonate != "" {
		headers["impersonate"] = item.Impersonate
	}
	return headers
}

func (s *AccountService) toPublicItems(items []model.Account) []dto.AccountPublic {
	results := make([]dto.AccountPublic, 0, len(items))
	for _, item := range items {
		results = append(results, dto.AccountPublic{
			ID:               item.PublicID(),
			AccessToken:      item.AccessToken,
			Type:             item.Type,
			Status:           item.Status,
			Quota:            item.Quota,
			Email:            item.Email,
			UserID:           item.UserID,
			LimitsProgress:   item.LimitsProgressValue(),
			DefaultModelSlug: item.DefaultModelSlug,
			RestoreAt:        item.RestoreAt,
			Success:          item.Success,
			Fail:             item.Fail,
			LastUsedAt:       formatTimePointer(item.LastUsedAt),
		})
	}
	return results
}

func normalizeAccountType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "free":
		return "Free"
	case "plus", "chatgptplus":
		return "Plus"
	case "pro":
		return "Pro"
	case "team", "business":
		return "Team"
	default:
		return firstNonEmpty(strings.TrimSpace(value), "Free")
	}
}

func searchAccountType(value any) string {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lowerKey := strings.ToLower(key)
			if strings.Contains(lowerKey, "plan") || strings.Contains(lowerKey, "type") || strings.Contains(lowerKey, "subscription") {
				if text := asString(child); text != "" {
					normalized := normalizeAccountType(text)
					if normalized != "Free" || strings.EqualFold(text, "free") {
						return normalized
					}
				}
			}
			if nested := searchAccountType(child); nested != "" {
				return nested
			}
		}
	case []any:
		for _, child := range current {
			if nested := searchAccountType(child); nested != "" {
				return nested
			}
		}
	case string:
		text := strings.TrimSpace(current)
		switch strings.ToLower(text) {
		case "free", "plus", "pro", "team", "business", "chatgptplus":
			return normalizeAccountType(text)
		}
	}
	return ""
}

func extractQuotaAndRestoreAt(items []map[string]any) (int, string) {
	quota := 0
	restoreAt := ""

	for _, item := range items {
		switch {
		case asInt(item["remaining"]) > 0:
			quota = asInt(item["remaining"])
		case asInt(item["limit"]) > 0:
			quota = asInt(item["limit"])
		case asInt(item["quota"]) > 0:
			quota = asInt(item["quota"])
		case asInt(item["max"]) > 0:
			quota = asInt(item["max"]) - asInt(item["current"])
		}
		if quota < 0 {
			quota = 0
		}

		restoreAt = firstNonEmpty(
			restoreAt,
			asString(item["restore_at"]),
			asString(item["reset_at"]),
			asString(item["reset_time"]),
		)

		if quota > 0 {
			return quota, restoreAt
		}
	}

	return quota, restoreAt
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func asString(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case json.Number:
		return current.String()
	case float64:
		return fmt.Sprintf("%.0f", current)
	default:
		return ""
	}
}

func asInt(value any) int {
	switch current := value.(type) {
	case int:
		return current
	case int64:
		return int(current)
	case float64:
		return int(current)
	case json.Number:
		n, _ := current.Int64()
		return int(n)
	default:
		return 0
	}
}
