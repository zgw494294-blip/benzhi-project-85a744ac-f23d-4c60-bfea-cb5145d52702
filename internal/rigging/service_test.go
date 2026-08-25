package rigging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"stage-rig-clearance/internal/audit"
	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

func TestFullClearanceFlowAndFrozenBoundary(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	auditor, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer auditor.Close()
	service := rigging.NewService(repo, auditor)
	show := time.Now().UTC().AddDate(0, 3, 0).Format("2006-01-02")
	cert := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	plan, err := service.CreatePlan(ctx, rigging.CreatePlanRequest{VenueName: "测试剧场", PerformanceDate: show, RatedTotalLoadKg: 2000, OwnerName: "技师"}, "test-create-001")
	if err != nil {
		t.Fatal(err)
	}
	plan, err = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{Label: "A", RatedLoadKg: 800, PlannedLoadKg: 500, DeviceModel: "H1", CableSpec: "C1", CertificateExpiresOn: cert}, "test-point-a")
	if err != nil {
		t.Fatal(err)
	}
	pointA := plan.Points[0].ID
	plan, err = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{Label: "B", RatedLoadKg: 800, PlannedLoadKg: 500, DeviceModel: "H2", CableSpec: "C2", PrimaryPointID: pointA, CertificateExpiresOn: cert}, "test-point-b")
	if err != nil {
		t.Fatal(err)
	}
	pointB := plan.Points[1].ID
	for index, pointID := range []string{pointA, pointB} {
		plan, err = service.RecordTest(ctx, plan.ID, plan.Version, passingTest(pointID, rigging.TestInitial), []string{"test-initial-a", "test-initial-b"}[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	var redundancyIssue string
	for _, issue := range plan.Issues {
		if issue.PointID == pointA && issue.RuleCode == "REDUNDANCY_MISSING" {
			redundancyIssue = issue.ID
		}
	}
	if redundancyIssue == "" {
		t.Fatal("missing deterministic redundancy issue")
	}
	plan, err = service.Remediate(ctx, plan.ID, plan.Version, rigging.RemediateRequest{IssueID: redundancyIssue, Note: "增加 B 冗余", RevisedBy: "技师", RedundantPointID: &pointB}, "test-remediate")
	if err != nil {
		t.Fatal(err)
	}
	plan, err = service.RecordTest(ctx, plan.ID, plan.Version, passingTest(pointA, rigging.TestRetest), "test-retest-a")
	if err != nil {
		t.Fatal(err)
	}
	if open := countOpen(plan); open != 0 {
		t.Fatalf("open issues = %d", open)
	}
	plan, err = service.Review(ctx, plan.ID, plan.Version, rigging.ReviewRequest{Decision: "approve", Reviewer: "安全负责人", Note: "通过"}, "test-review")
	if err != nil {
		t.Fatal(err)
	}
	frozenVersion := plan.Version
	_, err = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{Label: "C"}, "test-frozen-write")
	if !errors.Is(err, rigging.ErrFrozen) {
		t.Fatalf("expected ErrFrozen, got %v", err)
	}
	plan, err = service.IssueCredential(ctx, plan.ID, frozenVersion, "安全负责人", "test-credential")
	if err != nil {
		t.Fatal(err)
	}
	credential, verification, err := service.VerifyCredential(ctx, plan.ID, plan.Credentials[0].ID)
	if err != nil || !verification.Valid || credential.FrozenDigest != plan.FrozenDigest {
		t.Fatalf("credential verification failed: %+v %v", verification, err)
	}
	timeline, chain, err := service.Timeline(ctx, plan.ID)
	if err != nil || !chain.Valid || len(timeline) < 9 {
		t.Fatalf("invalid timeline: len=%d verification=%+v err=%v", len(timeline), chain, err)
	}
}

func TestExpectedVersionAndIdempotentRetry(t *testing.T) {
	repo, _ := store.Open(t.TempDir())
	defer repo.Close()
	auditor, _ := audit.Open(t.TempDir())
	defer auditor.Close()
	service := rigging.NewService(repo, auditor)
	request := rigging.CreatePlanRequest{VenueName: "版本测试", PerformanceDate: "2030-01-01", RatedTotalLoadKg: 1000, OwnerName: "负责人"}
	first, err := service.CreatePlan(context.Background(), request, "stable-create-key")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := service.CreatePlan(context.Background(), request, "stable-create-key")
	if err != nil || retry.ID != first.ID || retry.Version != first.Version {
		t.Fatalf("idempotent retry mismatch: %v", err)
	}
	_, err = service.AddPoint(context.Background(), first.ID, first.Version+1, rigging.AddPointRequest{}, "conflict-point-key")
	if !errors.Is(err, rigging.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func passingTest(pointID string, kind rigging.TestKind) rigging.RecordTestRequest {
	return rigging.RecordTestRequest{PointID: pointID, TestKind: kind, TargetLoadKg: 625, MeasuredLoadKg: 630, HoldSeconds: 90, DeformationMm: .5, Outcome: rigging.OutcomePass, EvidenceDigest: "sha256:test", PerformedBy: "试验员"}
}

func countOpen(plan *rigging.RigPlan) int {
	count := 0
	for _, issue := range plan.Issues {
		if issue.Status != rigging.IssueClosed {
			count++
		}
	}
	return count
}
