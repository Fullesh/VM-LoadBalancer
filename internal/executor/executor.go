package executor

import (
	"VM-LoadBalancer/internal/planner"
	"context"
	"log"
)

type Executor interface {
	Migrate(ctx context.Context, plan planner.MigrationPlan) error
}

type DryRunExecutor struct{}

func (DryRunExecutor) Migrate(ctx context.Context, plan planner.MigrationPlan) error {
	log.Printf("[DRY-RUN] запланирована миграция %s: %s -> %s (%s)",
		plan.VMName, plan.FromHost, plan.ToHost, plan.Reason)
	return nil
}
