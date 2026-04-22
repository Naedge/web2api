package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"web2api/internal/dto"
	"web2api/internal/model"
	"web2api/internal/repository"
)

type CPAService struct {
	repo           *repository.CPAPoolRepository
	accountService *AccountService
	tlsVerify      bool
}

type CPAImportJob struct {
	JobID     string              `json:"job_id"`
	Status    string              `json:"status"`
	CreatedAt string              `json:"created_at"`
	UpdatedAt string              `json:"updated_at"`
	Total     int                 `json:"total"`
	Completed int                 `json:"completed"`
	Added     int                 `json:"added"`
	Skipped   int                 `json:"skipped"`
	Refreshed int                 `json:"refreshed"`
	Failed    int                 `json:"failed"`
	Errors    []map[string]string `json:"errors"`
}

func NewCPAService(
	repo *repository.CPAPoolRepository,
	accountService *AccountService,
	tlsVerify bool,
) *CPAService {
	return &CPAService{
		repo:           repo,
		accountService: accountService,
		tlsVerify:      tlsVerify,
	}
}

func (s *CPAService) ListPools(ctx context.Context) ([]map[string]any, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		results = append(results, s.publicPool(item))
	}
	return results, nil
}

func (s *CPAService) CreatePool(ctx context.Context, req dto.CPAPoolCreateRequest) (map[string]any, error) {
	if strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.SecretKey) == "" {
		return nil, badRequest("base_url and secret_key are required")
	}

	item := &model.CPAPool{
		PoolID:    "pool_" + newHexID(16),
		Name:      strings.TrimSpace(req.Name),
		BaseURL:   strings.TrimRight(strings.TrimSpace(req.BaseURL), "/"),
		SecretKey: strings.TrimSpace(req.SecretKey),
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.publicPool(*item), nil
}

func (s *CPAService) UpdatePool(
	ctx context.Context,
	poolID string,
	req dto.CPAPoolUpdateRequest,
) (map[string]any, error) {
	item, err := s.repo.GetByPoolID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, notFound("pool not found")
	}

	if req.Name != nil {
		item.Name = strings.TrimSpace(*req.Name)
	}
	if req.BaseURL != nil {
		item.BaseURL = strings.TrimRight(strings.TrimSpace(*req.BaseURL), "/")
	}
	if req.SecretKey != nil {
		item.SecretKey = strings.TrimSpace(*req.SecretKey)
	}
	if err := s.repo.Save(ctx, item); err != nil {
		return nil, err
	}
	return s.publicPool(*item), nil
}

func (s *CPAService) DeletePool(ctx context.Context, poolID string) error {
	rows, err := s.repo.DeleteByPoolID(ctx, poolID)
	if err != nil {
		return err
	}
	if rows == 0 {
		return notFound("pool not found")
	}
	return nil
}

func (s *CPAService) ListRemoteFiles(ctx context.Context, poolID string) ([]map[string]any, error) {
	item, err := s.repo.GetByPoolID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, notFound("pool not found")
	}

	payload, err := s.fetchJSON(ctx, *item, http.MethodGet, "/v0/management/auth-files", nil)
	if err != nil {
		return nil, err
	}

	switch current := payload.(type) {
	case []any:
		results := make([]map[string]any, 0, len(current))
		for _, entry := range current {
			if mapped, ok := entry.(map[string]any); ok {
				results = append(results, normalizeRemoteFile(mapped))
			}
		}
		return results, nil
	case map[string]any:
		if files, ok := current["files"].([]any); ok {
			results := make([]map[string]any, 0, len(files))
			for _, entry := range files {
				if mapped, ok := entry.(map[string]any); ok {
					results = append(results, normalizeRemoteFile(mapped))
				}
			}
			return results, nil
		}
	}

	return []map[string]any{}, nil
}

func (s *CPAService) StartImport(ctx context.Context, poolID string, names []string) (*CPAImportJob, error) {
	item, err := s.repo.GetByPoolID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, notFound("pool not found")
	}

	names = normalizeStringList(names)
	if len(names) == 0 {
		return nil, badRequest("selected files is required")
	}

	job := s.loadImportJob(item.ImportJobJSON)
	if job.Status == "pending" || job.Status == "running" {
		return nil, &StatusError{Code: 409, Message: "import job is already running"}
	}

	job = newImportJob(len(names))
	if err := s.saveImportJob(ctx, item, job); err != nil {
		return nil, err
	}

	go s.runImport(poolID, names)
	return &job, nil
}

func (s *CPAService) GetImportJob(ctx context.Context, poolID string) (*CPAImportJob, error) {
	item, err := s.repo.GetByPoolID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, notFound("pool not found")
	}

	job := s.loadImportJob(item.ImportJobJSON)
	if job.JobID == "" {
		return nil, nil
	}
	return &job, nil
}

func (s *CPAService) runImport(poolID string, names []string) {
	ctx := context.Background()

	item, err := s.repo.GetByPoolID(ctx, poolID)
	if err != nil || item == nil {
		return
	}

	job := s.loadImportJob(item.ImportJobJSON)
	job.Status = "running"
	job.UpdatedAt = nowISO()
	_ = s.saveImportJob(ctx, item, job)

	_, err = s.ListRemoteFiles(ctx, poolID)
	if err != nil {
		job.Status = "failed"
		job.UpdatedAt = nowISO()
		job.Failed = len(job.Errors)
		if job.Failed == 0 {
			job.Failed = 1
			job.Errors = append(job.Errors, map[string]string{
				"name":  "",
				"error": err.Error(),
			})
		}
		_ = s.saveImportJob(ctx, item, job)
		return
	}

	tokens := []string{}
	for _, name := range names {
		payload, err := s.fetchRaw(ctx, *item, http.MethodGet, "/v0/management/auth-files/"+name)
		if err != nil {
			job.Errors = append(job.Errors, map[string]string{
				"name":  name,
				"error": err.Error(),
			})
			job.Failed = len(job.Errors)
			job.Completed++
			job.UpdatedAt = nowISO()
			_ = s.saveImportJob(ctx, item, job)
			continue
		}

		if text := extractFileContent(payload); text != "" {
			for _, token := range strings.Split(text, "\n") {
				token = strings.TrimSpace(token)
				if token != "" {
					tokens = append(tokens, token)
				}
			}
		}
		job.Completed++
		job.UpdatedAt = nowISO()
		_ = s.saveImportJob(ctx, item, job)
	}

	if len(tokens) == 0 {
		job.Status = "failed"
		job.UpdatedAt = nowISO()
		job.Failed = len(job.Errors)
		_ = s.saveImportJob(ctx, item, job)
		return
	}

	_, added, skipped, err := s.accountService.AddTokens(ctx, tokens)
	if err != nil {
		job.Status = "failed"
		job.UpdatedAt = nowISO()
		job.Errors = append(job.Errors, map[string]string{
			"name":  "",
			"error": err.Error(),
		})
		job.Failed = len(job.Errors)
		_ = s.saveImportJob(ctx, item, job)
		return
	}

	refreshResult, err := s.accountService.RefreshAccountsDetailed(ctx, tokens)
	if err != nil {
		job.Errors = append(job.Errors, map[string]string{
			"name":  "",
			"error": err.Error(),
		})
	}
	for _, refreshErr := range refreshResult.Errors {
		job.Errors = append(job.Errors, map[string]string{
			"name":  refreshErr.AccessToken,
			"error": refreshErr.Error,
		})
	}

	job.Status = "completed"
	job.UpdatedAt = nowISO()
	job.Added = added
	job.Skipped = skipped
	job.Refreshed = refreshResult.Refreshed
	job.Failed = len(job.Errors)
	job.Completed = len(names)
	job.Total = len(names)
	_ = s.saveImportJob(ctx, item, job)
}

func (s *CPAService) publicPool(item model.CPAPool) map[string]any {
	payload := map[string]any{
		"id":       item.PoolID,
		"name":     item.Name,
		"base_url": item.BaseURL,
	}
	job := s.loadImportJob(item.ImportJobJSON)
	if job.JobID != "" {
		payload["import_job"] = job
	}
	return payload
}

func (s *CPAService) loadImportJob(raw string) CPAImportJob {
	if strings.TrimSpace(raw) == "" {
		return CPAImportJob{}
	}
	job := CPAImportJob{}
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		return CPAImportJob{}
	}
	if job.JobID == "" {
		job.JobID = newHexID(24)
	}
	if job.Status == "" {
		job.Status = "failed"
	}
	if job.CreatedAt == "" {
		job.CreatedAt = nowISO()
	}
	if job.UpdatedAt == "" {
		job.UpdatedAt = job.CreatedAt
	}
	if job.Errors == nil {
		job.Errors = []map[string]string{}
	}
	return job
}

func (s *CPAService) saveImportJob(ctx context.Context, item *model.CPAPool, job CPAImportJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	item.ImportJobJSON = string(data)
	return s.repo.Save(ctx, item)
}

func (s *CPAService) fetchJSON(
	ctx context.Context,
	pool model.CPAPool,
	method string,
	path string,
	body any,
) (any, error) {
	payload, err := s.fetchRaw(ctx, pool, method, path)
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, badGateway(fmt.Sprintf("invalid json response: %v", err))
	}
	return result, nil
}

func (s *CPAService) fetchRaw(ctx context.Context, pool model.CPAPool, method string, path string) ([]byte, error) {
	client, err := newHTTPClient(s.tlsVerify, 30*time.Second, nil)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		method,
		strings.TrimRight(pool.BaseURL, "/")+path,
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+pool.SecretKey)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, badGateway(firstNonEmpty(strings.TrimSpace(string(data)), resp.Status))
	}
	return data, nil
}

func extractFileContent(payload []byte) string {
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err == nil {
		return firstNonEmpty(asString(result["content"]), asString(result["contents"]))
	}
	return string(payload)
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func newImportJob(total int) CPAImportJob {
	now := nowISO()
	return CPAImportJob{
		JobID:     newHexID(24),
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
		Total:     total,
		Completed: 0,
		Added:     0,
		Skipped:   0,
		Refreshed: 0,
		Failed:    0,
		Errors:    []map[string]string{},
	}
}

func normalizeRemoteFile(item map[string]any) map[string]any {
	return map[string]any{
		"name":  asString(item["name"]),
		"email": firstNonEmpty(asString(item["email"]), asString(item["account"])),
	}
}
