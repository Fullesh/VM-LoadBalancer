package analyzer

import (
	"VM-LoadBalancer/model"
	"cmp"
	"slices"
)

type Config struct {
	HighThresholdPercent float64
	MinSkewPercent       float64
}

type Report struct {
	NeedsRebalance bool
	MaxLoadPercent float64
	MinLoadPercent float64
	AvgLoadPercent float64
	HotHosts       []model.Hypervisor
	ColdHosts      []model.Hypervisor
}

type Analyzer struct {
	cfg Config
}

func New(cfg Config) *Analyzer {
	return &Analyzer{cfg: cfg}
}

func (a *Analyzer) Analyze(hosts []model.Hypervisor) Report {
	if len(hosts) < 2 {
		return Report{NeedsRebalance: false}
	}
	var minLoad, maxLoad, sumLoad float64
	for i, h := range hosts {
		load := h.MaxUsagePercent()
		if i == 0 {
			minLoad = load
			maxLoad = load
		} else {
			minLoad = min(minLoad, load)
			maxLoad = max(maxLoad, load)
		}
		sumLoad += load
	}
	avgLoad := sumLoad / float64(len(hosts))
	needsRebalance := maxLoad >= a.cfg.HighThresholdPercent && (maxLoad-minLoad) >= a.cfg.MinSkewPercent
	var hotHosts, coldHosts []model.Hypervisor
	if needsRebalance {
		for i, h := range hosts {
			if h.MaxUsagePercent() >= a.cfg.HighThresholdPercent {
				hotHosts = append(hotHosts, hosts[i])
			}
			if h.MaxUsagePercent() < avgLoad {
				coldHosts = append(coldHosts, hosts[i])
			}
		}
	}
	slices.SortFunc(hotHosts, func(a, b model.Hypervisor) int {
		return cmp.Compare(b.MaxUsagePercent(), a.MaxUsagePercent())
	})
	slices.SortFunc(coldHosts, func(a, b model.Hypervisor) int {
		return cmp.Compare(a.MaxUsagePercent(), b.MaxUsagePercent())
	})
	return Report{NeedsRebalance: needsRebalance, MaxLoadPercent: maxLoad, MinLoadPercent: minLoad, AvgLoadPercent: avgLoad, HotHosts: hotHosts, ColdHosts: coldHosts}
}
