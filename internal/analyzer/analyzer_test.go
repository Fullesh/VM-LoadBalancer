package analyzer

import (
	"VM-LoadBalancer/model"
	"testing"
)

func TestAnalyze_TooFewHosts(t *testing.T) {
	a := New(Config{HighThresholdPercent: 70, MinSkewPercent: 25})

	report := a.Analyze([]model.Hypervisor{
		{Name: "hv-01", CpuUsageCores: 15, CpuCapacityCores: 16},
	})
	if report.NeedsRebalance {
		t.Fatalf("для одного хоста ожидалось NeedsRebalance=false, получено true")
	}
}

func TestAnalyze_BalancedCluster(t *testing.T) {
	a := New(Config{HighThresholdPercent: 70, MinSkewPercent: 25})

	hosts := []model.Hypervisor{
		{Name: "hv-01", CpuUsageCores: 8, CpuCapacityCores: 16, MemoryUsageBytes: 32, MemoryCapacityBytes: 64},  // 50%
		{Name: "hv-02", CpuUsageCores: 7, CpuCapacityCores: 16, MemoryUsageBytes: 28, MemoryCapacityBytes: 64},  // 43.75%
		{Name: "hv-03", CpuUsageCores: 10, CpuCapacityCores: 16, MemoryUsageBytes: 30, MemoryCapacityBytes: 64}, // 62.5%
	}

	report := a.Analyze(hosts)
	if report.NeedsRebalance {
		t.Fatalf("ожидался сбалансированный кластер, получено NeedsRebalance=true (max=%.1f, min=%.1f)",
			report.MaxLoadPercent, report.MinLoadPercent)
	}
}

func TestAnalyze_HighLoadButLowSkew(t *testing.T) {
	a := New(Config{HighThresholdPercent: 70, MinSkewPercent: 25})

	// Все хосты нагружены примерно одинаково — переносить некуда.
	hosts := []model.Hypervisor{
		{Name: "hv-01", CpuUsageCores: 13, CpuCapacityCores: 16}, // 81.25%
		{Name: "hv-02", CpuUsageCores: 12, CpuCapacityCores: 16}, // 75.0%
	}

	report := a.Analyze(hosts)
	if report.NeedsRebalance {
		t.Fatalf("ожидалось NeedsRebalance=false, так как разброс меньше MinSkewPercent")
	}
}

func TestAnalyze_SkewedClusterSorting(t *testing.T) {
	a := New(Config{HighThresholdPercent: 70, MinSkewPercent: 25})

	hosts := []model.Hypervisor{
		{Name: "hv-02", CpuUsageCores: 3, CpuCapacityCores: 16, MemoryUsageBytes: 12, MemoryCapacityBytes: 64},  // 18.75%
		{Name: "hv-01", CpuUsageCores: 13, CpuCapacityCores: 16, MemoryUsageBytes: 46, MemoryCapacityBytes: 64}, // 81.25%
		{Name: "hv-04", CpuUsageCores: 15, CpuCapacityCores: 16, MemoryUsageBytes: 40, MemoryCapacityBytes: 64}, // 93.75%
		{Name: "hv-03", CpuUsageCores: 1, CpuCapacityCores: 16, MemoryUsageBytes: 4, MemoryCapacityBytes: 64},   // 6.25%
	}

	report := a.Analyze(hosts)
	if !report.NeedsRebalance {
		t.Fatalf("ожидалось NeedsRebalance=true")
	}

	if len(report.HotHosts) != 2 {
		t.Fatalf("ожидалось 2 перегруженных хоста, получено %d", len(report.HotHosts))
	}
	if report.HotHosts[0].Name != "hv-04" || report.HotHosts[1].Name != "hv-01" {
		t.Fatalf("неверный порядок HotHosts: %v, %v", report.HotHosts[0].Name, report.HotHosts[1].Name)
	}

	if len(report.ColdHosts) != 2 {
		t.Fatalf("ожидалось 2 недогруженных хоста, получено %d", len(report.ColdHosts))
	}
	if report.ColdHosts[0].Name != "hv-03" || report.ColdHosts[1].Name != "hv-02" {
		t.Fatalf("неверный порядок ColdHosts: %v, %v", report.ColdHosts[0].Name, report.ColdHosts[1].Name)
	}
}
