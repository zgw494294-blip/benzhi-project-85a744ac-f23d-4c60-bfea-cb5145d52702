package batch_cancel_ghost_commit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"stage-rig-clearance/internal/rigging"
)

type controlledRepository struct {
	mu        sync.Mutex
	plan      *rigging.RigPlan
	entered   chan struct{}
	release   chan struct{}
	finished  chan struct{}
	committed bool
}

func newControlledRepository(plan *rigging.RigPlan) *controlledRepository {
	return &controlledRepository{
		plan: plan, entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{}),
	}
}

func (r *controlledRepository) Get(context.Context, string) (*rigging.RigPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyValue := *r.plan
	copyValue.Points = append([]rigging.SuspensionPoint(nil), r.plan.Points...)
	return &copyValue, nil
}

func (r *controlledRepository) List(context.Context) ([]*rigging.RigPlan, error) {
	return nil, errors.New("unexpected List call")
}

func (r *controlledRepository) LookupCommand(context.Context, string, string) (*rigging.CommandReceipt, error) {
	return nil, rigging.ErrNotFound
}

func (r *controlledRepository) Commit(ctx context.Context, _ *rigging.RigPlan, _ int64, _ []rigging.DomainEvent, _, _ string, _ rigging.CommandReceipt) error {
	close(r.entered)
	defer close(r.finished)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		r.mu.Lock()
		r.committed = true
		r.mu.Unlock()
		return nil
	}
}

func (r *controlledRepository) FindCredential(context.Context, string) (*rigging.RigPlan, rigging.ClearanceCredential, error) {
	return nil, rigging.ClearanceCredential{}, errors.New("unexpected FindCredential call")
}

func (r *controlledRepository) didCommit() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.committed
}

func TestCanceledBatchDoesNotCommitInBackground(t *testing.T) {
	point := rigging.SuspensionPoint{
		ID: "point-1", PlanID: "plan-1", Label: "主吊点", RatedLoadKg: 800, PlannedLoadKg: 400,
		DeviceModel: "HOIST-X", CableSpec: "12mm", CertificateExpiresOn: "2030-01-01", ConfigRevision: 1,
	}
	point.ConfigDigest = rigging.PointConfigDigest(point)
	plan := &rigging.RigPlan{
		ID: "plan-1", VenueName: "实验剧场", PerformanceDate: "2027-01-01", RatedTotalLoadKg: 1200,
		OwnerName: "技师甲", Status: rigging.StatusDraft, Version: 1, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Points: []rigging.SuspensionPoint{point},
	}
	repository := newControlledRepository(plan)
	service := rigging.NewService(repository, nil)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := service.RecordTestBatch(ctx, plan.ID, plan.Version, rigging.RecordTestBatchRequest{Tests: []rigging.RecordTestRequest{{
			PointID: point.ID, TestKind: rigging.TestInitial, TargetLoadKg: 500, MeasuredLoadKg: 505,
			HoldSeconds: 60, DeformationMm: 0.3, Outcome: rigging.OutcomePass, EvidenceDigest: "evidence-1",
			PerformedBy: "试验员乙", PointConfigDigest: point.ConfigDigest,
		}}}, "batch-cancel-key")
		result <- err
	}()

	<-repository.entered
	cancel()
	err := <-result
	close(repository.release)
	<-repository.finished

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消批量归档应返回 context.Canceled，实际为 %v", err)
	}
	if repository.didCommit() {
		t.Fatal("调用方已经观察到取消错误，但 Repository.Commit 仍在后台完成了批量归档")
	}
}
