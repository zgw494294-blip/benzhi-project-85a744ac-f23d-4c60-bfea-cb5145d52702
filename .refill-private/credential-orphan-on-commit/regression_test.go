package credential_orphan_on_commit_test

import (
	"context"
	"testing"
	"time"

	"stage-rig-clearance/internal/audit"
	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

func TestCredentialCommitFailureLeavesNoSealedAuditFact(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	plan := &rigging.RigPlan{ID: "approved-plan", VenueName: "凭据剧场", PerformanceDate: "2030-01-01", RatedTotalLoadKg: 1000, OwnerName: "负责人", Status: rigging.StatusApproved, Version: 1, CreatedAt: now, FrozenAt: &now, Points: []rigging.SuspensionPoint{}, Tests: []rigging.LoadTest{}, FrozenPoints: []rigging.SuspensionPoint{}, FrozenTests: []rigging.LoadTest{}}
	plan.FrozenDigest, err = rigging.FrozenDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	receipt := rigging.CommandReceipt{PlanID: plan.ID, Version: 1, ResourceID: plan.ID, Action: "plan.seeded"}
	if err := repo.Commit(context.Background(), plan, 0, []rigging.DomainEvent{{Type: "plan.seeded", Actor: "负责人"}}, "credential-seed-key", "plan.seeded", receipt); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	auditor, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer auditor.Close()
	service := rigging.NewService(repo, auditor)

	if _, err := service.IssueCredential(context.Background(), plan.ID, plan.Version, "负责人", "credential-failing-key"); err == nil {
		t.Fatal("closed repository did not fail credential domain commit")
	}
	records, verification, err := auditor.Timeline(context.Background(), plan.ID)
	if err != nil || !verification.Valid {
		t.Fatalf("audit inspection failed: verification=%+v err=%v", verification, err)
	}
	for _, record := range records {
		if record.Action == "credential.sealed" {
			t.Fatalf("failed credential command left sealed audit record sequence=%d", record.Sequence)
		}
	}
}
