package domain_audit_half_commit_test

import (
	"context"
	"testing"

	"stage-rig-clearance/internal/audit"
	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

func TestAuditFailureDoesNotExposeCommittedDomainMutation(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	auditor, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := rigging.NewService(repo, auditor)
	plan, err := service.CreatePlan(context.Background(), rigging.CreatePlanRequest{VenueName: "半提交剧场", PerformanceDate: "2030-01-01", RatedTotalLoadKg: 1000, OwnerName: "技师"}, "half-create-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := auditor.Close(); err != nil {
		t.Fatal(err)
	}

	_, commandErr := service.AddPoint(context.Background(), plan.ID, plan.Version, rigging.AddPointRequest{Label: "A", RatedLoadKg: 500, PlannedLoadKg: 300, DeviceModel: "H1", CableSpec: "C1", CertificateExpiresOn: "2031-01-01"}, "half-point-key")
	if commandErr == nil {
		t.Fatal("closed audit store did not fail the command")
	}
	stored, err := service.GetPlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != plan.Version || len(stored.Points) != 0 {
		t.Fatalf("failed audited command remained committed: version=%d points=%d commandErr=%v", stored.Version, len(stored.Points), commandErr)
	}
}
