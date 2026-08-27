package snapshot_log_misbinding_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

func TestSnapshotMustBelongToValidatedEventLog(t *testing.T) {
	primaryDir := filepath.Join(t.TempDir(), "primary")
	foreignDir := filepath.Join(t.TempDir(), "foreign")
	commitPlan(t, primaryDir, "plan-primary", "主日志方案", "primary-key")
	commitPlan(t, foreignDir, "plan-foreign", "外来快照方案", "foreign-key")

	foreignSnapshot, err := os.ReadFile(filepath.Join(foreignDir, "projection.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primaryDir, "projection.json"), foreignSnapshot, 0o640); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(primaryDir)
	if err != nil {
		t.Fatalf("事件日志完整且应能独立恢复: %v", err)
	}
	defer reopened.Close()
	plan, err := reopened.Get(context.Background(), "plan-primary")
	if err != nil {
		t.Fatalf("主事件日志中的已提交方案在重启后消失: %v", err)
	}
	if plan.VenueName != "主日志方案" {
		t.Fatalf("恢复了错误快照中的方案: %+v", plan)
	}
	if _, err := reopened.Get(context.Background(), "plan-foreign"); err == nil {
		t.Fatal("与事件日志无关的外来快照污染了恢复投影")
	}
}

func commitPlan(t *testing.T, dir, id, venue, key string) {
	t.Helper()
	repo, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	plan := &rigging.RigPlan{
		ID: id, VenueName: venue, PerformanceDate: "2031-06-01", RatedTotalLoadKg: 800,
		OwnerName: "恢复测试员", Status: rigging.StatusDraft, Version: 1, CreatedAt: time.Unix(1_900_000_000, 0).UTC(),
	}
	receipt := rigging.CommandReceipt{PlanID: id, Version: 1, ResourceID: id, Action: "plan.created"}
	events := []rigging.DomainEvent{{Type: "plan.created", Actor: "恢复测试员", Payload: map[string]any{"venueName": venue}}}
	if err := repo.Commit(context.Background(), plan, 0, events, key, "plan.created", receipt); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
}
