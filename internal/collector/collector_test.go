package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

type mockResult struct {
	Metric map[string]string `json:"metric"`
	Value  [2]any            `json:"value"`
}

func TestVMClient_Collect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("query")

		var results []mockResult
		switch {
		case q == "node_cpu_cores_total":
			results = []mockResult{
				{Metric: map[string]string{"host": "hv-01"}, Value: [2]any{1700000000, "16"}},
				{Metric: map[string]string{"host": "hv-02"}, Value: [2]any{1700000000, "16"}},
			}
		case q == "node_memory_total_bytes":
			results = []mockResult{
				{Metric: map[string]string{"host": "hv-01"}, Value: [2]any{1700000000, "64"}},
				{Metric: map[string]string{"host": "hv-02"}, Value: [2]any{1700000000, "64"}},
			}
		case q == "libvirt_domain_info_virtual_cpus":
			results = []mockResult{
				{Metric: map[string]string{"domain": "vm-1", "host": "hv-01"}, Value: [2]any{1700000000, "4"}},
				{Metric: map[string]string{"domain": "vm-2", "host": "hv-02"}, Value: [2]any{1700000000, "2"}},
			}
		case q == "libvirt_domain_info_memory_usage_bytes":
			results = []mockResult{
				{Metric: map[string]string{"domain": "vm-1", "host": "hv-01"}, Value: [2]any{1700000000, "12"}},
				{Metric: map[string]string{"domain": "vm-2", "host": "hv-02"}, Value: [2]any{1700000000, "4"}},
			}
		case q == "libvirt_domain_info_maximum_memory_bytes":
			results = []mockResult{
				{Metric: map[string]string{"domain": "vm-1", "host": "hv-01"}, Value: [2]any{1700000000, "16"}},
				{Metric: map[string]string{"domain": "vm-2", "host": "hv-02"}, Value: [2]any{1700000000, "8"}},
			}
		case strings.Contains(q, "rate(libvirt_domain_info_cpu_time_seconds_total"):
			// Проверяем, что работает агрегация по domain без лейбла host.
			results = []mockResult{
				{Metric: map[string]string{"domain": "vm-1"}, Value: [2]any{1700000000, "3.5"}},
				{Metric: map[string]string{"domain": "vm-2"}, Value: [2]any{1700000000, "1.0"}},
			}
		default:
			t.Fatalf("неожиданный PromQL-запрос: %q", q)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result":     results,
			},
		})
	}))
	defer srv.Close()

	client := NewVMClient(srv.URL)
	hosts, vms, err := client.Collect(context.Background())
	if err != nil {
		t.Fatalf("неожиданная ошибка Collect(): %v", err)
	}

	if len(hosts) != 2 || len(vms) != 2 {
		t.Fatalf("ожидалось 2 хоста и 2 VM, получено %d хостов и %d VM", len(hosts), len(vms))
	}

	for _, h := range hosts {
		switch h.Name {
		case "hv-01":
			if h.CpuUsageCores != 3.5 || h.MemoryUsageBytes != 12 || !slices.Equal(h.MachinesInside, []string{"vm-1"}) {
				t.Fatalf("неожиданное состояние hv-01: %+v", h)
			}
		case "hv-02":
			if h.CpuUsageCores != 1.0 || h.MemoryUsageBytes != 4 || !slices.Equal(h.MachinesInside, []string{"vm-2"}) {
				t.Fatalf("неожиданное состояние hv-02: %+v", h)
			}
		default:
			t.Fatalf("неизвестный хост: %s", h.Name)
		}
	}
}

func TestVMClient_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewVMClient(srv.URL)
	if _, _, err := client.Collect(context.Background()); err == nil {
		t.Fatalf("ожидалась ошибка при HTTP 500, получено nil")
	}
}
