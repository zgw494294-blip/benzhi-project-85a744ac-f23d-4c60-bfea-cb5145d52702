package verification_context_crosstalk_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"stage-rig-clearance/internal/rigging"
)

func TestConcurrentVerificationDoesNotInheritPeerCancellation(t *testing.T) {
	plan := &rigging.RigPlan{
		ID: "plan-context-owner", VenueName: "上下文边界剧场", PerformanceDate: "2030-08-25",
		RatedTotalLoadKg: 1000, Status: rigging.StatusApproved,
		Points: []rigging.SuspensionPoint{}, Tests: []rigging.LoadTest{},
		FrozenPoints: []rigging.SuspensionPoint{}, FrozenTests: []rigging.LoadTest{},
	}
	frozenDigest, err := rigging.FrozenDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.FrozenDigest = frozenDigest
	credential := rigging.ClearanceCredential{
		ID: "clearance-context-owner", PlanID: plan.ID, Sequence: 1,
		FrozenDigest: frozenDigest, CredentialDigest: strings.Repeat("a", 64),
	}
	repo := &blockingRepository{
		plan: plan, credential: credential,
		firstStarted: make(chan struct{}), secondStarted: make(chan struct{}),
	}
	service := rigging.NewService(repo, validAuditor{credentialID: credential.ID})

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	leaderErr := make(chan error, 1)
	go func() {
		_, callErr := service.VerifyCredentialGlobally(leaderCtx, credential.ID, credential.CredentialDigest)
		leaderErr <- callErr
	}()
	<-repo.firstStarted

	probe := &doneProbeContext{
		Context: context.Background(), observed: make(chan struct{}), never: make(chan struct{}),
	}
	type outcome struct {
		verification rigging.GlobalCredentialVerification
		err          error
	}
	followerDone := make(chan outcome, 1)
	go func() {
		verification, callErr := service.VerifyCredentialGlobally(probe, credential.ID, credential.CredentialDigest)
		followerDone <- outcome{verification: verification, err: callErr}
	}()

	select {
	case <-probe.observed:
		// 当前实现已让跟随请求等待首请求拥有的共享调用。
	case <-repo.secondStarted:
		// 正确实现会让仍有效的请求拥有独立的存储调用。
	}
	cancelLeader()

	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("首请求取消后返回 %v，期望 context.Canceled", err)
	}
	follower := <-followerDone
	if follower.err != nil {
		t.Fatalf("仍有效的并发核验继承了另一请求的取消错误: %v", follower.err)
	}
	if !follower.verification.Valid {
		t.Fatalf("仍有效的并发核验没有得到有效结果: %+v", follower.verification)
	}
}

type doneProbeContext struct {
	context.Context
	once     sync.Once
	observed chan struct{}
	never    chan struct{}
}

func (c *doneProbeContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.never
}

type blockingRepository struct {
	mu            sync.Mutex
	calls         int
	plan          *rigging.RigPlan
	credential    rigging.ClearanceCredential
	firstStarted  chan struct{}
	secondStarted chan struct{}
}

func (r *blockingRepository) FindCredential(ctx context.Context, _ string) (*rigging.RigPlan, rigging.ClearanceCredential, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.firstStarted)
		<-ctx.Done()
		return nil, rigging.ClearanceCredential{}, ctx.Err()
	}
	if call == 2 {
		close(r.secondStarted)
	}
	return r.plan, r.credential, nil
}

func (r *blockingRepository) Get(context.Context, string) (*rigging.RigPlan, error) {
	return nil, rigging.ErrNotFound
}
func (r *blockingRepository) List(context.Context) ([]*rigging.RigPlan, error) {
	return nil, rigging.ErrNotFound
}
func (r *blockingRepository) LookupCommand(context.Context, string, string) (*rigging.CommandReceipt, error) {
	return nil, rigging.ErrNotFound
}
func (r *blockingRepository) Commit(context.Context, *rigging.RigPlan, int64, []rigging.DomainEvent, string, string, rigging.CommandReceipt) error {
	return errors.New("unexpected Commit")
}

type validAuditor struct {
	credentialID string
}

func (a validAuditor) Timeline(context.Context, string) ([]rigging.AuditRecord, rigging.Verification, error) {
	records := []rigging.AuditRecord{{Action: "credential.sealed", Payload: map[string]any{"credentialId": a.credentialID}}}
	return records, rigging.Verification{Valid: true}, nil
}
func (validAuditor) VerifyCredential(context.Context, rigging.ClearanceCredential) (rigging.Verification, error) {
	return rigging.Verification{Valid: true}, nil
}
func (validAuditor) Append(context.Context, string, string, string, map[string]any) (rigging.AuditRecord, error) {
	return rigging.AuditRecord{}, errors.New("unexpected Append")
}
func (validAuditor) Issue(context.Context, string, string, string) (rigging.ClearanceCredential, error) {
	return rigging.ClearanceCredential{}, errors.New("unexpected Issue")
}
