package timeline_cache_stale_after_command_test

import (
	"context"
	"sync"
	"testing"

	"stage-rig-clearance/internal/audit"
	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

type blockingTimelineAuditor struct {
	rigging.Auditor
	snapshotTaken chan struct{}
	release       chan struct{}
	once          sync.Once
}

func (a *blockingTimelineAuditor) Timeline(ctx context.Context, planID string) ([]rigging.AuditRecord, rigging.Verification, error) {
	records, verification, err := a.Auditor.Timeline(ctx, planID)
	a.once.Do(func() {
		close(a.snapshotTaken)
		<-a.release
	})
	return records, verification, err
}

type timelineResult struct {
	records      []rigging.AuditRecord
	verification rigging.Verification
	err          error
}

func TestTimelineCacheObservesLaterAuditAppend(t *testing.T) {
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

	blockingAudit := &blockingTimelineAuditor{
		Auditor: auditor, snapshotTaken: make(chan struct{}), release: make(chan struct{}),
	}
	service := rigging.NewService(repo, blockingAudit)
	plan, err := service.CreatePlan(ctx, rigging.CreatePlanRequest{
		VenueName: "缓存复现剧场", PerformanceDate: "2030-08-25",
		RatedTotalLoadKg: 1200, OwnerName: "技师甲",
	}, "timeline-create-001")
	if err != nil {
		t.Fatal(err)
	}
	timelineDone := make(chan timelineResult, 1)
	go func() {
		records, verification, timelineErr := service.Timeline(ctx, plan.ID)
		timelineDone <- timelineResult{records: records, verification: verification, err: timelineErr}
	}()
	<-blockingAudit.snapshotTaken

	plan, err = service.AddPoint(ctx, plan.ID, plan.Version, rigging.AddPointRequest{
		Label: "主吊点 A", RatedLoadKg: 800, PlannedLoadKg: 500,
		DeviceModel: "H-800", CableSpec: "8mm-6x19",
		CertificateExpiresOn: "2031-08-25",
	}, "timeline-point-001")
	close(blockingAudit.release)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version != 2 {
		t.Fatalf("point command did not commit: version=%d", plan.Version)
	}
	initial := <-timelineDone
	if initial.err != nil || !initial.verification.Valid || len(initial.records) != 1 {
		t.Fatalf("controlled timeline snapshot failed: records=%d verification=%+v err=%v", len(initial.records), initial.verification, initial.err)
	}

	records, verification, err := service.Timeline(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Valid {
		t.Fatalf("audit chain unexpectedly invalid: %+v", verification)
	}
	if len(records) != 2 || records[1].Action != "point.added" {
		t.Fatalf("timeline cache hid later audit append: records=%+v", records)
	}
}
