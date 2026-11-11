package model

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestHypervisorUsagePercent(t *testing.T) {
	hv := Hypervisor{
		Name:                "hv-01",
		CpuUsageCores:       12,
		CpuCapacityCores:    16,
		MemoryUsageBytes:    32,
		MemoryCapacityBytes: 64,
	}

	if got := hv.CpuUsagePercent(); !almostEqual(got, 75.0) {
		t.Fatalf("CpuUsagePercent() = %v, ожидалось 75.0", got)
	}
	if got := hv.MemoryUsagePercent(); !almostEqual(got, 50.0) {
		t.Fatalf("MemoryUsagePercent() = %v, ожидалось 50.0", got)
	}
	if got := hv.MaxUsagePercent(); !almostEqual(got, 75.0) {
		t.Fatalf("MaxUsagePercent() = %v, ожидалось 75.0", got)
	}
}

func TestZeroCapacityProtection(t *testing.T) {
	hv := Hypervisor{
		CpuUsageCores:       4,
		CpuCapacityCores:    0,
		MemoryUsageBytes:    1024,
		MemoryCapacityBytes: -1,
	}
	if got := hv.CpuUsagePercent(); got != 0 {
		t.Fatalf("при нулевой ёмкости CPU ожидалось 0, получено %v", got)
	}
	if got := hv.MemoryUsagePercent(); got != 0 {
		t.Fatalf("при отрицательной ёмкости RAM ожидалось 0, получено %v", got)
	}

	vm := VirtualMachine{
		CpuUsageCores:       2,
		CpuCapacityCores:    0,
		MemoryUsageBytes:    512,
		MemoryCapacityBytes: 0,
	}
	if got := vm.MaxUsagePercent(); got != 0 {
		t.Fatalf("при нулевой ёмкости VM ожидалось 0, получено %v", got)
	}
}

func TestVirtualMachineUsagePercent(t *testing.T) {
	vm := VirtualMachine{
		Name:                "vm-1",
		CpuUsageCores:       2,
		CpuCapacityCores:    4,
		MemoryUsageBytes:    12,
		MemoryCapacityBytes: 16,
	}

	if got := vm.CpuUsagePercent(); !almostEqual(got, 50.0) {
		t.Fatalf("CpuUsagePercent() = %v, ожидалось 50.0", got)
	}
	if got := vm.MemoryUsagePercent(); !almostEqual(got, 75.0) {
		t.Fatalf("MemoryUsagePercent() = %v, ожидалось 75.0", got)
	}
	if got := vm.MaxUsagePercent(); !almostEqual(got, 75.0) {
		t.Fatalf("MaxUsagePercent() = %v, ожидалось 75.0", got)
	}
}
