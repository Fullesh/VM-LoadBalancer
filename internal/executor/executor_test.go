package executor

import (
	"VM-LoadBalancer/internal/planner"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDryRunExecutor_Migrate(t *testing.T) {
	exec := DryRunExecutor{}
	err := exec.Migrate(context.Background(), planner.MigrationPlan{
		VMName:   "vm-db-1",
		FromHost: "hv-01",
		ToHost:   "hv-03",
		Reason:   "hv-01: 81.2% -> 43.8%, hv-03: 6.2% -> 43.8%",
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestLabExecutor_MigrateSuccess(t *testing.T) {
	var got struct {
		VM     string `json:"vm"`
		Target string `json:"target"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/migrate" {
			t.Fatalf("неожиданный HTTP-запрос: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("ошибка декодирования тела запроса: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exec := NewLabExecutor(srv.URL)
	err := exec.Migrate(context.Background(), planner.MigrationPlan{
		VMName:   "vm-db-1",
		FromHost: "hv-01",
		ToHost:   "hv-03",
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got.VM != "vm-db-1" || got.Target != "hv-03" {
		t.Fatalf("отправлен неверный JSON-запрос: %+v", got)
	}
}

func TestLabExecutor_MigrateErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "недостаточно RAM", http.StatusBadRequest)
	}))
	defer srv.Close()

	exec := NewLabExecutor(srv.URL)
	err := exec.Migrate(context.Background(), planner.MigrationPlan{
		VMName: "vm-db-1",
		ToHost: "hv-03",
	})
	if err == nil {
		t.Fatalf("ожидалась ошибка при HTTP 400, получено nil")
	}
}
