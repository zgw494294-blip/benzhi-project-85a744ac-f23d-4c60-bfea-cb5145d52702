package rigging_test

import (
	"context"
	"errors"
	"testing"

	"stage-rig-clearance/internal/audit"
	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

func TestPlanAndPointRevisionBoundaries(t *testing.T) {
	ctx := context.Background()
	repo, _ := store.Open(t.TempDir())
	defer repo.Close()
	auditor, _ := audit.Open(t.TempDir())
	defer auditor.Close()
	service := rigging.NewService(repo, auditor)
	plan, err := service.CreatePlan(ctx, rigging.CreatePlanRequest{VenueName: "修订剧场", PerformanceDate: "2030-01-01", RatedTotalLoadKg: 1000, OwnerName: "技师"}, "extension-create-01")
	if err != nil {
		t.Fatal(err)
	}
	plan, err = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{Label: "A", RatedLoadKg: 800, PlannedLoadKg: 500, DeviceModel: "H1", CableSpec: "C1", CertificateExpiresOn: "2031-01-01"}, "extension-point-01")
	if err != nil {
		t.Fatal(err)
	}
	pointID := plan.Points[0].ID
	plan, err = service.RecordTest(ctx, plan.ID, plan.Version, passingTest(pointID, rigging.TestInitial), "extension-test-01")
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := plan.Version
	update := rigging.UpdatePlanRequest{VenueName: plan.VenueName, PerformanceDate: "2032-01-01", RatedTotalLoadKg: plan.RatedTotalLoadKg, OwnerName: plan.OwnerName}
	plan, err = service.UpdatePlan(ctx, plan.ID, oldVersion, update, "extension-plan-update")
	if err != nil || plan.Version != oldVersion+1 || len(plan.Tests) != 1 || len(plan.PlanRevisions) != 1 {
		t.Fatalf("plan revision result: version=%d tests=%d revisions=%d err=%v", plan.Version, len(plan.Tests), len(plan.PlanRevisions), err)
	}
	if !hasIssue(plan, pointID, "CERTIFICATE_EXPIRED", rigging.IssueOpen) {
		t.Fatal("performance date revision did not reopen certificate risk")
	}
	retry, err := service.UpdatePlan(ctx, plan.ID, oldVersion, update, "extension-plan-update")
	if err != nil || retry.Version != plan.Version {
		t.Fatalf("idempotent plan retry changed result: %v", err)
	}
	_, err = service.UpdatePlan(ctx, plan.ID, oldVersion, rigging.UpdatePlanRequest{VenueName: "过期写入", PerformanceDate: "2032-01-01", RatedTotalLoadKg: 1000, OwnerName: "技师"}, "extension-plan-stale")
	if !errors.Is(err, rigging.ErrVersionConflict) {
		t.Fatalf("expected stale plan version conflict, got %v", err)
	}
	point := plan.Points[0]
	pointUpdate := rigging.UpdatePointRequest{Label: point.Label, RatedLoadKg: point.RatedLoadKg, PlannedLoadKg: 550, DeviceModel: point.DeviceModel, CableSpec: point.CableSpec, PrimaryPointID: point.PrimaryPointID, RedundantPointID: point.RedundantPointID, CertificateExpiresOn: point.CertificateExpiresOn}
	plan, err = service.UpdatePoint(ctx, plan.ID, pointID, plan.Version, pointUpdate, "extension-point-update")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Points[0].ConfigRevision != 2 || plan.Tests[0].CurrentConfiguration || !hasIssue(plan, pointID, "TEST_MISSING", rigging.IssueOpen) {
		t.Fatalf("point revision did not invalidate old test: %+v %+v", plan.Points[0], plan.Tests[0])
	}
	_, err = service.RemovePoint(ctx, plan.ID, pointID, plan.Version, "extension-remove-history")
	if !errors.Is(err, rigging.ErrStateConflict) {
		t.Fatalf("point with test history was removable: %v", err)
	}
	plan, err = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{Label: "临时吊点", RatedLoadKg: 100, PlannedLoadKg: 50, DeviceModel: "H2", CableSpec: "C2", CertificateExpiresOn: "2033-01-01"}, "extension-temp-point")
	if err != nil {
		t.Fatal(err)
	}
	temporaryID := plan.Points[len(plan.Points)-1].ID
	check, err := service.CheckPointRemoval(ctx, plan.ID, temporaryID)
	if err != nil || !check.Allowed {
		t.Fatalf("fresh point removal check failed: %+v %v", check, err)
	}
	plan, err = service.RemovePoint(ctx, plan.ID, temporaryID, plan.Version, "extension-remove-fresh")
	if err != nil || len(plan.Points) != 1 {
		t.Fatalf("fresh point was not removed: %v", err)
	}
}

func TestBatchAtomicityAndRetestTaskPrecision(t *testing.T) {
	ctx := context.Background()
	repo, _ := store.Open(t.TempDir())
	defer repo.Close()
	auditor, _ := audit.Open(t.TempDir())
	defer auditor.Close()
	service := rigging.NewService(repo, auditor)
	plan, _ := service.CreatePlan(ctx, rigging.CreatePlanRequest{VenueName: "批量剧场", PerformanceDate: "2030-01-01", RatedTotalLoadKg: 2000, OwnerName: "技师"}, "batch-create-01")
	plan, _ = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{Label: "A", RatedLoadKg: 800, PlannedLoadKg: 500, DeviceModel: "H1", CableSpec: "C1", CertificateExpiresOn: "2031-01-01"}, "batch-point-a")
	pointA := plan.Points[0].ID
	plan, _ = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{Label: "B", RatedLoadKg: 800, PlannedLoadKg: 500, DeviceModel: "H2", CableSpec: "C2", PrimaryPointID: pointA, CertificateExpiresOn: "2031-01-01"}, "batch-point-b")
	pointB := plan.Points[1].ID
	invalid := rigging.RecordTestBatchRequest{Tests: []rigging.RecordTestRequest{batchTest(plan.Points[0], 90), batchTest(plan.Points[1], 0)}}
	version := plan.Version
	_, err := service.RecordTestBatch(ctx, plan.ID, version, invalid, "batch-invalid-key")
	if !errors.Is(err, rigging.ErrValidation) {
		t.Fatalf("invalid batch accepted: %v", err)
	}
	unchanged, _ := service.GetPlan(ctx, plan.ID)
	if unchanged.Version != version || len(unchanged.Tests) != 0 {
		t.Fatalf("invalid batch partially wrote: version=%d tests=%d", unchanged.Version, len(unchanged.Tests))
	}
	valid := rigging.RecordTestBatchRequest{Tests: []rigging.RecordTestRequest{batchTest(plan.Points[0], 90), batchTest(plan.Points[1], 90)}}
	result, err := service.RecordTestBatch(ctx, plan.ID, version, valid, "batch-valid-key")
	if err != nil || result.Plan.Version != version+1 || len(result.Tests) != 2 || result.Tests[0].TestID == result.Tests[1].TestID {
		t.Fatalf("valid batch result invalid: %+v err=%v", result, err)
	}
	retry, err := service.RecordTestBatch(ctx, plan.ID, version, valid, "batch-valid-key")
	if err != nil || retry.BatchID != result.BatchID || len(retry.Plan.Tests) != 2 {
		t.Fatalf("batch retry duplicated facts: %+v err=%v", retry, err)
	}
	plan = result.Plan
	var issueID string
	for _, issue := range plan.Issues {
		if issue.PointID == pointA && issue.RuleCode == "REDUNDANCY_MISSING" {
			issueID = issue.ID
		}
	}
	plan, err = service.Remediate(ctx, plan.ID, plan.Version, rigging.RemediateRequest{IssueID: issueID, Note: "绑定 B 冗余", RevisedBy: "技师", RedundantPointID: &pointB}, "batch-remediate")
	if err != nil || len(plan.RetestTasks) != 1 {
		t.Fatalf("retest task was not created: %v", err)
	}
	task := plan.RetestTasks[0]
	wrong := passingTest(pointB, rigging.TestRetest)
	wrong.RetestTaskID = task.ID
	_, err = service.RecordTest(ctx, plan.ID, plan.Version, wrong, "batch-wrong-retest")
	if !errors.Is(err, rigging.ErrStateConflict) {
		t.Fatalf("task accepted another point: %v", err)
	}
	failed := passingTest(pointA, rigging.TestRetest)
	failed.RetestTaskID, failed.HoldSeconds = task.ID, 10
	plan, err = service.RecordTest(ctx, plan.ID, plan.Version, failed, "batch-failed-retest")
	if err != nil || len(plan.RetestTasks[0].AttemptTestIDs) != 1 || plan.RetestTasks[0].Status != rigging.RetestTaskPending {
		t.Fatalf("failed retest attempt was not retained: %+v err=%v", plan.RetestTasks[0], err)
	}
	passed := passingTest(pointA, rigging.TestRetest)
	passed.RetestTaskID = task.ID
	plan, err = service.RecordTest(ctx, plan.ID, plan.Version, passed, "batch-passed-retest")
	if err != nil || plan.RetestTasks[0].Status != rigging.RetestTaskClosed || !hasIssue(plan, pointA, "REDUNDANCY_MISSING", rigging.IssueClosed) {
		t.Fatalf("bound risk was not precisely closed: %+v err=%v", plan.RetestTasks[0], err)
	}
}

func batchTest(point rigging.SuspensionPoint, hold int) rigging.RecordTestRequest {
	return rigging.RecordTestRequest{PointID: point.ID, TestKind: rigging.TestInitial, TargetLoadKg: point.PlannedLoadKg, MeasuredLoadKg: point.PlannedLoadKg + 5, HoldSeconds: hold, DeformationMm: .5, Outcome: rigging.OutcomePass, EvidenceDigest: "sha256:batch", PerformedBy: "试验员", PointConfigDigest: point.ConfigDigest}
}

func hasIssue(plan *rigging.RigPlan, pointID, code string, status rigging.IssueStatus) bool {
	for _, issue := range plan.Issues {
		if issue.PointID == pointID && issue.RuleCode == code && issue.Status == status {
			return true
		}
	}
	return false
}
