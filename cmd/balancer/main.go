package main

import (
	"VM-LoadBalancer/internal/analyzer"
	"VM-LoadBalancer/internal/collector"
	"VM-LoadBalancer/internal/executor"
	"VM-LoadBalancer/internal/planner"
	"VM-LoadBalancer/model"
	"cmp"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"
)

const GiB = 1024 * 1024 * 1024

func main() {
	vmURL := flag.String("vm-url", "http://localhost:8428", "VictoriaMetrics URL")
	labURL := flag.String("lab-url", "http://localhost:9177", "Lab generator URL")
	dryRun := flag.Bool("dry-run", false, "Log migrations without executing them")
	interval := flag.Duration("interval", 15*time.Second, "Reconcile interval")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	col := collector.NewVMClient(*vmURL)
	anl := analyzer.New(analyzer.Config{
		HighThresholdPercent: 70,
		MinSkewPercent:       25,
	})
	pln := planner.New(planner.Config{
		TargetMaxPercent: 70,
		MinGainPercent:   10,
		Cooldown:         45 * time.Second,
	})

	var exec executor.Executor
	if *dryRun {
		exec = executor.DryRunExecutor{}
	} else {
		exec = executor.NewLabExecutor(*labURL)
	}

	log.Printf("Балансировщик запущен (interval=%s, dry-run=%v)", *interval, *dryRun)
	reconcile(ctx, col, anl, pln, exec)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Остановка балансировщика...")
			return
		case <-ticker.C:
			reconcile(ctx, col, anl, pln, exec)
		}
	}
}

func reconcile(
	ctx context.Context,
	col *collector.VMClient,
	anl *analyzer.Analyzer,
	pln *planner.Planner,
	exec executor.Executor,
) {
	stepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	hosts, vms, err := col.Collect(stepCtx)
	if err != nil {
		log.Printf("[ERROR] ошибка сбора метрик: %v", err)
		return
	}
	if len(hosts) == 0 {
		log.Println("[WARN] метрики кластера пока не получены")
		return
	}

	slices.SortFunc(hosts, func(a, b model.Hypervisor) int {
		return cmp.Compare(a.Name, b.Name)
	})

	log.Println("--- Состояние кластера ---")
	for _, h := range hosts {
		slices.Sort(h.MachinesInside)
		log.Printf("  %-6s | CPU: %5.1f%% (%4.1f/%.0f cores) | RAM: %5.1f%% (%4.1f/%.0f GiB) | VMs: %v",
			h.Name,
			h.CpuUsagePercent(), h.CpuUsageCores, h.CpuCapacityCores,
			h.MemoryUsagePercent(), h.MemoryUsageBytes/GiB, h.MemoryCapacityBytes/GiB,
			h.MachinesInside,
		)
	}

	report := anl.Analyze(hosts)
	if !report.NeedsRebalance {
		log.Printf("[OK] Кластер сбалансирован (max=%.1f%%, min=%.1f%%, avg=%.1f%%)",
			report.MaxLoadPercent, report.MinLoadPercent, report.AvgLoadPercent)
		return
	}

	log.Printf("[WARN] Обнаружен перекос (max=%.1f%%, min=%.1f%%, skew=%.1f%%)",
		report.MaxLoadPercent, report.MinLoadPercent, report.MaxLoadPercent-report.MinLoadPercent)

	plan, ok := pln.Plan(report, vms)
	if !ok {
		log.Println("[INFO] Безопасный перенос не найден (cooldown или риск перегрузки приемника)")
		return
	}

	if err := exec.Migrate(stepCtx, plan); err != nil {
		log.Printf("[ERROR] ошибка миграции %s (%s -> %s): %v", plan.VMName, plan.FromHost, plan.ToHost, err)
		return
	}

	pln.MarkMigrated(plan.VMName)
	log.Printf("[MIGRATED] %s: %s -> %s (%s)", plan.VMName, plan.FromHost, plan.ToHost, plan.Reason)
}
