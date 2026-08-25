package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"stage-rig-clearance/internal/rigging"
)

func TestRepositoryRecoversProjectionAndReceipt(t *testing.T) {
	dir := t.TempDir()
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	plan := &rigging.RigPlan{ID: "plan-recover", VenueName: "恢复测试", PerformanceDate: "2030-01-01", RatedTotalLoadKg: 100, OwnerName: "甲", Status: rigging.StatusDraft, Version: 1, CreatedAt: time.Now().UTC()}
	receipt := rigging.CommandReceipt{PlanID: plan.ID, Version: 1, ResourceID: plan.ID, Action: "plan.created"}
	if err := repo.Commit(context.Background(), plan, 0, []rigging.DomainEvent{{Type: "plan.created", Actor: "甲"}}, "recover-key", "plan.created", receipt); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.Get(context.Background(), plan.ID)
	if err != nil || loaded.Version != 1 {
		t.Fatalf("projection recovery failed: %+v %v", loaded, err)
	}
	loadedReceipt, err := reopened.LookupCommand(context.Background(), "recover-key", "plan.created")
	if err != nil || loadedReceipt.ResourceID != plan.ID {
		t.Fatalf("receipt recovery failed: %+v %v", loadedReceipt, err)
	}
}

func TestRepositoryRejectsCorruptEventLog(t *testing.T) {
	dir := t.TempDir()
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	plan := &rigging.RigPlan{ID: "plan-corrupt", Version: 1, Status: rigging.StatusDraft, CreatedAt: time.Now().UTC()}
	receipt := rigging.CommandReceipt{PlanID: plan.ID, Version: 1, Action: "plan.created"}
	if err := repo.Commit(context.Background(), plan, 0, nil, "corrupt-key", "plan.created", receipt); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events.log")
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := file.Stat()
	if _, err := file.WriteAt([]byte{0xff}, info.Size()-2); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := Open(dir); err == nil {
		t.Fatal("corrupt event log was accepted")
	}
}
