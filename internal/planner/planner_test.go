package planner

import (
	"VM-LoadBalancer/internal/analyzer"
	"VM-LoadBalancer/model"
	"testing"
	"time"
)

func TestPlan_SelectsOptimalMigration(t *testing.T) {
	p := New(Config{
		TargetMaxPercent: 70,
		MinGainPercent:   10,
		Cooldown:         60 * time.Second,
	})

	hv1 := model.Hypervisor{
		Name:                "hv-01",
		MachinesInside:      []string{"vm-db-1", "vm-api-1"},
		CpuUsageCores:       10,
		CpuCapacityCores:    16, // 62.5%
		MemoryUsageBytes:    50,
		MemoryCapacityBytes: 64, // 78.125%
	}
	hv2 := model.Hypervisor{
		Name:                "hv-02",
		MachinesInside:      []string{"vm-web-1"},
		CpuUsageCores:       3,
		CpuCapacityCores:    16,
		MemoryUsageBytes:    12,
		MemoryCapacityBytes: 64,
	}
	hv3 := model.Hypervisor{
		Name:                "hv-03",
		MachinesInside:      []string{"vm-worker-1"},
		CpuUsageCores:       1,
		CpuCapacityCores:    16,
		MemoryUsageBytes:    4,
		MemoryCapacityBytes: 64,
	}

	vms := []model.VirtualMachine{
		{Name: "vm-db-1", HypervisorName: "hv-01", CpuUsageCores: 6, CpuCapacityCores: 8, MemoryUsageBytes: 24, MemoryCapacityBytes: 32},
		{Name: "vm-api-1", HypervisorName: "hv-01", CpuUsageCores: 4, CpuCapacityCores: 4, MemoryUsageBytes: 26, MemoryCapacityBytes: 32},
		{Name: "vm-web-1", HypervisorName: "hv-02", CpuUsageCores: 3, CpuCapacityCores: 4, MemoryUsageBytes: 12, MemoryCapacityBytes: 16},
		{Name: "vm-worker-1", HypervisorName: "hv-03", CpuUsageCores: 1, CpuCapacityCores: 2, MemoryUsageBytes: 4, MemoryCapacityBytes: 8},
	}

	report := analyzer.Report{
		NeedsRebalance: true,
		HotHosts:       []model.Hypervisor{hv1},
		ColdHosts:      []model.Hypervisor{hv3, hv2},
	}

	plan, ok := p.Plan(report, vms)
	if !ok {
		t.Fatalf("ожидался план миграции, получено ok=false")
	}
	if plan.VMName != "vm-db-1" || plan.FromHost != "hv-01" || plan.ToHost != "hv-03" {
		t.Fatalf("неожиданный план миграции: %+v", plan)
	}
}

func TestPlan_PreventsPingPongWithMinGain(t *testing.T) {
	p := New(Config{
		TargetMaxPercent: 85,
		MinGainPercent:   10,
		Cooldown:         60 * time.Second,
	})

	// hv-01 на 75%, hv-02 на 50%. Перенос VM на 31.25% сделает hv-02 = 81.25%,
	// что выше, чем было на hv-01 -> перекос только перевернётся.
	hv1 := model.Hypervisor{Name: "hv-01", MachinesInside: []string{"vm-big"}, CpuUsageCores: 12, CpuCapacityCores: 16}
	hv2 := model.Hypervisor{Name: "hv-02", CpuUsageCores: 8, CpuCapacityCores: 16}
	vms := []model.VirtualMachine{
		{Name: "vm-big", HypervisorName: "hv-01", CpuUsageCores: 5, CpuCapacityCores: 8},
	}

	report := analyzer.Report{
		NeedsRebalance: true,
		HotHosts:       []model.Hypervisor{hv1},
		ColdHosts:      []model.Hypervisor{hv2},
	}

	if plan, ok := p.Plan(report, vms); ok {
		t.Fatalf("миграция должна была быть заблокирована во избежание пинг-понга, но получен план: %+v", plan)
	}
}

func TestPlan_RespectsCooldown(t *testing.T) {
	p := New(Config{
		TargetMaxPercent: 70,
		MinGainPercent:   10,
		Cooldown:         1 * time.Minute,
	})

	hv1 := model.Hypervisor{
		Name:                "hv-01",
		MachinesInside:      []string{"vm-db-1", "vm-api-1"},
		CpuUsageCores:       13,
		CpuCapacityCores:    16,
		MemoryUsageBytes:    46,
		MemoryCapacityBytes: 64,
	}
	hv3 := model.Hypervisor{
		Name:                "hv-03",
		CpuUsageCores:       1,
		CpuCapacityCores:    16,
		MemoryUsageBytes:    4,
		MemoryCapacityBytes: 64,
	}
	vms := []model.VirtualMachine{
		{Name: "vm-db-1", HypervisorName: "hv-01", CpuUsageCores: 6, CpuCapacityCores: 8, MemoryUsageBytes: 24, MemoryCapacityBytes: 32},
		{Name: "vm-api-1", HypervisorName: "hv-01", CpuUsageCores: 4, CpuCapacityCores: 4, MemoryUsageBytes: 12, MemoryCapacityBytes: 16},
	}
	report := analyzer.Report{
		NeedsRebalance: true,
		HotHosts:       []model.Hypervisor{hv1},
		ColdHosts:      []model.Hypervisor{hv3},
	}

	p.MarkMigrated("vm-db-1")

	plan, ok := p.Plan(report, vms)
	if !ok {
		t.Fatalf("ожидался выбор запасной vm-api-1, пока vm-db-1 на кулдауне")
	}
	if plan.VMName != "vm-api-1" {
		t.Fatalf("ожидалась vm-api-1, получена %s", plan.VMName)
	}

	p.MarkMigrated("vm-api-1")
	if _, ok := p.Plan(report, vms); ok {
		t.Fatalf("не ожидалось никакого плана, когда все кандидаты на кулдауне")
	}
}
