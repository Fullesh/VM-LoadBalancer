package model

type VirtualMachine struct {
	Name                string
	HypervisorName      string
	CpuUsageCores       float64
	CpuCapacityCores    float64
	MemoryUsageBytes    float64
	MemoryCapacityBytes float64
}

type Hypervisor struct {
	Name                string
	MachinesInside      []string
	CpuUsageCores       float64
	CpuCapacityCores    float64
	MemoryUsageBytes    float64
	MemoryCapacityBytes float64
}

func (vm VirtualMachine) CpuUsagePercent() float64 {
	if vm.CpuCapacityCores <= 0 {
		return 0.0
	}
	return (vm.CpuUsageCores / vm.CpuCapacityCores) * 100
}

func (vm VirtualMachine) MemoryUsagePercent() float64 {
	if vm.MemoryCapacityBytes <= 0 {
		return 0.0
	}
	return (vm.MemoryUsageBytes / vm.MemoryCapacityBytes) * 100
}

func (h Hypervisor) CpuUsagePercent() float64 {
	if h.CpuCapacityCores <= 0 {
		return 0.0
	}
	return h.CpuUsageCores / h.CpuCapacityCores * 100
}

func (h Hypervisor) MemoryUsagePercent() float64 {
	if h.MemoryCapacityBytes <= 0 {
		return 0.0
	}
	return h.MemoryUsageBytes / h.MemoryCapacityBytes * 100
}

func (h Hypervisor) MaxUsagePercent() float64 {
	return max(h.CpuUsagePercent(), h.MemoryUsagePercent())
}

func (vm VirtualMachine) MaxUsagePercent() float64 {
	return max(vm.CpuUsagePercent(), vm.MemoryUsagePercent())
}
