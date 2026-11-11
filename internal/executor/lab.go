package executor

import (
	"VM-LoadBalancer/internal/planner"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type LabExecutor struct {
	baseURL    string
	httpClient *http.Client
}

func NewLabExecutor(baseURL string) *LabExecutor {
	return &LabExecutor{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (e *LabExecutor) Migrate(ctx context.Context, plan planner.MigrationPlan) error {
	payload := struct {
		VM     string `json:"vm"`
		Target string `json:"target"`
	}{
		VM:     plan.VMName,
		Target: plan.ToHost,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ошибка сериализации JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/migrate", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ошибка создания HTTP-запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения HTTP-запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ошибка миграции (статус %d): %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	return nil
}
