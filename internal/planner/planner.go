package planner

import (
	"VM-LoadBalancer/internal/analyzer"
	"VM-LoadBalancer/model"
	"fmt"
	"sync"
	"time"
)

type MigrationPlan struct {
	VMName   string
	FromHost string
	ToHost   string
	Reason   string
}

type Config struct {
	TargetMaxPercent float64
	MinGainPercent   float64
	Cooldown         time.Duration
}

type Planner struct {
	mu           sync.Mutex
	cfg          Config
	lastMigrated map[string]time.Time
}

func New(cfg Config) *Planner {
	lastMigrated := make(map[string]time.Time)
	return &Planner{
		cfg:          cfg,
		lastMigrated: lastMigrated,
	}
}

func (p *Planner) MarkMigrated(vmName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastMigrated[vmName] = time.Now()
}

// мини-хелпер определяющий не на кулдауне ли сейчас vm
func (p *Planner) isOnCooldown(vmName string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lastTime, ok := p.lastMigrated[vmName]; ok && time.Since(lastTime) < p.cfg.Cooldown {
		return true
	}
	return false
}

// мини-хелпер для оценки переноса
func (p *Planner) evaluateMove(vm model.VirtualMachine, src, dst model.Hypervisor) (float64, string, bool) {
	newDst := dst
	newDst.CpuUsageCores += vm.CpuUsageCores
	newDst.MemoryUsageBytes += vm.MemoryUsageBytes
	newDstLoad := newDst.MaxUsagePercent()

	newSrc := src
	newSrc.CpuUsageCores -= vm.CpuUsageCores
	newSrc.MemoryUsageBytes -= vm.MemoryUsageBytes
	newSrcLoad := newSrc.MaxUsagePercent()

	if newDst.MemoryUsageBytes > newDst.MemoryCapacityBytes ||
		newDst.CpuUsageCores > newDst.CpuCapacityCores {
		return 0, "", false
	}

	if newDstLoad > p.cfg.TargetMaxPercent {
		return 0, "", false
	}

	if src.MaxUsagePercent()-newDstLoad < p.cfg.MinGainPercent {
		return 0, "", false
	}

	afterPeak := max(newSrcLoad, newDstLoad)
	reason := fmt.Sprintf("%s: %.1f%% -> %.1f%%, %s: %.1f%% -> %.1f%%",
		src.Name, src.MaxUsagePercent(), newSrcLoad,
		dst.Name, dst.MaxUsagePercent(), newDstLoad)
	return afterPeak, reason, true
}

func (p *Planner) Plan(report analyzer.Report, vms []model.VirtualMachine) (MigrationPlan, bool) {
	if !report.NeedsRebalance || len(report.HotHosts) == 0 || len(report.ColdHosts) == 0 {
		return MigrationPlan{}, false
	}
	vmMap := make(map[string]model.VirtualMachine, len(vms))
	for _, vm := range vms {
		vmMap[vm.Name] = vm
	}

	var (
		found    bool
		bestPeak float64
		bestPlan MigrationPlan
	)

	for _, src := range report.HotHosts {
		for _, vmName := range src.MachinesInside {
			if p.isOnCooldown(vmName) {
				continue
			}
			vm, ok := vmMap[vmName]
			if !ok {
				continue
			}
			for _, dst := range report.ColdHosts {
				if dst.Name == src.Name {
					continue
				}
				afterPeak, reason, ok := p.evaluateMove(vm, src, dst)
				if !ok {
					continue
				}
				if !found || afterPeak < bestPeak {
					found = true
					bestPeak = afterPeak
					bestPlan = MigrationPlan{
						VMName:   vm.Name,
						FromHost: src.Name,
						ToHost:   dst.Name,
						Reason:   reason,
					}
				}
			}
		}
	}
	return bestPlan, found
}
