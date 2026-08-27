package concurrenttimelinebuffer_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"stage-rig-clearance/internal/httpapi"
	"stage-rig-clearance/internal/rigging"
)

type staticRepository struct {
	plans map[string]*rigging.RigPlan
}

func (r *staticRepository) Get(_ context.Context, id string) (*rigging.RigPlan, error) {
	plan, ok := r.plans[id]
	if !ok {
		return nil, rigging.ErrNotFound
	}
	copy := *plan
	return &copy, nil
}

func (r *staticRepository) List(context.Context) ([]*rigging.RigPlan, error) {
	return nil, nil
}

func (r *staticRepository) LookupCommand(context.Context, string, string) (*rigging.CommandReceipt, error) {
	return nil, rigging.ErrNotFound
}

func (r *staticRepository) Commit(context.Context, *rigging.RigPlan, int64, []rigging.DomainEvent, string, string, rigging.CommandReceipt) error {
	return nil
}

func (r *staticRepository) FindCredential(context.Context, string) (*rigging.RigPlan, rigging.ClearanceCredential, error) {
	return nil, rigging.ClearanceCredential{}, rigging.ErrNotFound
}

type staticAuditor struct{}

func (staticAuditor) Append(context.Context, string, string, string, map[string]any) (rigging.AuditRecord, error) {
	return rigging.AuditRecord{}, nil
}

func (staticAuditor) Timeline(_ context.Context, planID string) ([]rigging.AuditRecord, rigging.Verification, error) {
	if planID == "plan-first" {
		return []rigging.AuditRecord{{PlanID: planID, Action: "first-marker", Payload: map[string]any{"blob": strings.Repeat("a", 32768)}}}, rigging.Verification{Valid: true}, nil
	}
	return []rigging.AuditRecord{{PlanID: planID, Action: "second-marker", Payload: map[string]any{"blob": "b"}}}, rigging.Verification{Valid: true}, nil
}

func (staticAuditor) Issue(context.Context, string, string, string) (rigging.ClearanceCredential, error) {
	return rigging.ClearanceCredential{}, nil
}

func (staticAuditor) VerifyCredential(context.Context, rigging.ClearanceCredential) (rigging.Verification, error) {
	return rigging.Verification{Valid: true}, nil
}

type blockedResponseWriter struct {
	header  http.Header
	body    bytes.Buffer
	status  int
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockedResponseWriter) Header() http.Header {
	return w.header
}

func (w *blockedResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *blockedResponseWriter) Write(payload []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.body.Write(payload)
}

func TestConcurrentTimelineResponsesOwnTheirEncodedBytes(t *testing.T) {
	repo := &staticRepository{plans: map[string]*rigging.RigPlan{
		"plan-first":  {ID: "plan-first"},
		"plan-second": {ID: "plan-second"},
	}}
	handler := httpapi.New(rigging.NewService(repo, staticAuditor{})).Handler()
	firstWriter := &blockedResponseWriter{
		header: make(http.Header), entered: make(chan struct{}), release: make(chan struct{}),
	}
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(firstWriter, httptest.NewRequest(http.MethodGet, "/api/plans/plan-first/audit", nil))
		close(firstDone)
	}()

	<-firstWriter.entered
	secondWriter := httptest.NewRecorder()
	handler.ServeHTTP(secondWriter, httptest.NewRequest(http.MethodGet, "/api/plans/plan-second/audit", nil))
	close(firstWriter.release)
	<-firstDone

	if firstWriter.status != http.StatusOK {
		t.Fatalf("first timeline status = %d, want %d", firstWriter.status, http.StatusOK)
	}
	if bytes.Contains(firstWriter.body.Bytes(), []byte("second-marker")) || !bytes.Contains(firstWriter.body.Bytes(), []byte("first-marker")) {
		t.Fatalf("first timeline response was overwritten by the concurrent second request")
	}
}
