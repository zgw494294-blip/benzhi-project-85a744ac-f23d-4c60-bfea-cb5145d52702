package credential_lookup_cache_race_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"stage-rig-clearance/internal/rigging"
)

type rendezvousRepository struct {
	plan       *rigging.RigPlan
	credential rigging.ClearanceCredential
	arrived    chan struct{}
	release    chan struct{}
}

func (r *rendezvousRepository) Get(context.Context, string) (*rigging.RigPlan, error) {
	return nil, rigging.ErrNotFound
}

func (r *rendezvousRepository) List(context.Context) ([]*rigging.RigPlan, error) {
	return nil, nil
}

func (r *rendezvousRepository) LookupCommand(context.Context, string, string) (*rigging.CommandReceipt, error) {
	return nil, rigging.ErrNotFound
}

func (r *rendezvousRepository) Commit(context.Context, *rigging.RigPlan, int64, []rigging.DomainEvent, string, string, rigging.CommandReceipt) error {
	return errors.New("unexpected commit")
}

func (r *rendezvousRepository) FindCredential(context.Context, string) (*rigging.RigPlan, rigging.ClearanceCredential, error) {
	r.arrived <- struct{}{}
	<-r.release
	return r.plan, r.credential, nil
}

type validAuditor struct {
	credentialID string
}

func (a validAuditor) Append(context.Context, string, string, string, map[string]any) (rigging.AuditRecord, error) {
	return rigging.AuditRecord{}, errors.New("unexpected append")
}

func (a validAuditor) Timeline(context.Context, string) ([]rigging.AuditRecord, rigging.Verification, error) {
	record := rigging.AuditRecord{Action: "credential.sealed", Payload: map[string]any{"credentialId": a.credentialID}}
	return []rigging.AuditRecord{record}, rigging.Verification{Valid: true}, nil
}

func (a validAuditor) Issue(context.Context, string, string, string) (rigging.ClearanceCredential, error) {
	return rigging.ClearanceCredential{}, errors.New("unexpected issue")
}

func (a validAuditor) VerifyCredential(context.Context, rigging.ClearanceCredential) (rigging.Verification, error) {
	return rigging.Verification{Valid: true}, nil
}

func TestConcurrentCredentialLookupCacheIsSynchronized(t *testing.T) {
	plan := &rigging.RigPlan{
		ID:               "plan-cache-race",
		VenueName:        "并发核验剧场",
		PerformanceDate:  "2032-08-25",
		RatedTotalLoadKg: 1200,
		FrozenPoints:     []rigging.SuspensionPoint{},
		FrozenTests:      []rigging.LoadTest{},
	}
	frozenDigest, err := rigging.FrozenProjectionDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.FrozenDigest = frozenDigest
	credential := rigging.ClearanceCredential{
		ID:               "clearance-cache-race",
		PlanID:           plan.ID,
		Sequence:         7,
		FrozenDigest:     frozenDigest,
		IssuedBy:         "安全负责人",
		IssuedAt:         time.Date(2032, 8, 25, 10, 0, 0, 0, time.UTC),
		CredentialDigest: strings.Repeat("a", 64),
	}
	repo := &rendezvousRepository{
		plan: plan, credential: credential,
		arrived: make(chan struct{}), release: make(chan struct{}),
	}
	service := rigging.NewService(repo, validAuditor{credentialID: credential.ID})
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			verification, verifyErr := service.VerifyCredentialGlobally(context.Background(), credential.ID, credential.CredentialDigest)
			if verifyErr == nil && !verification.Valid {
				verifyErr = errors.New("credential unexpectedly invalid")
			}
			results <- verifyErr
		}()
	}
	close(start)
	<-repo.arrived
	<-repo.arrived
	close(repo.release)
	for i := 0; i < 2; i++ {
		if verifyErr := <-results; verifyErr != nil {
			t.Fatal(verifyErr)
		}
	}
}
