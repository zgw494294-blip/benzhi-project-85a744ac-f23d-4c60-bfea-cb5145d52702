package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"stage-rig-clearance/internal/audit"
	"stage-rig-clearance/internal/httpapi"
	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

func TestFrontendAndStableProblemResponses(t *testing.T) {
	repo, _ := store.Open(t.TempDir())
	defer repo.Close()
	auditor, _ := audit.Open(t.TempDir())
	defer auditor.Close()
	handler := httpapi.New(rigging.NewService(repo, auditor)).Handler()
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), "<body>") || !strings.Contains(index.Body.String(), "舞台吊挂") {
		t.Fatalf("frontend unavailable: %d", index.Code)
	}
	createBody := map[string]any{"idempotencyKey": "http-create-key", "venueName": "HTTP 剧场", "performanceDate": "2030-01-01", "ratedTotalLoadKg": 1000, "ownerName": "技师"}
	created := performJSON(handler, http.MethodPost, "/api/plans", createBody)
	if created.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", created.Code, created.Body.String())
	}
	var plan rigging.RigPlan
	if err := json.Unmarshal(created.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	pointBody := map[string]any{"idempotencyKey": "http-point-key", "expectedVersion": plan.Version + 1, "label": "A", "ratedLoadKg": 100, "plannedLoadKg": 80, "deviceModel": "H", "cableSpec": "C", "primaryPointId": "", "redundantPointId": "", "certificateExpiresOn": "2031-01-01"}
	conflict := performJSON(handler, http.MethodPost, "/api/plans/"+plan.ID+"/points", pointBody)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "version_conflict") {
		t.Fatalf("unexpected conflict mapping: %d %s", conflict.Code, conflict.Body.String())
	}
	createBody["unknownField"] = true
	unknown := performJSON(handler, http.MethodPost, "/api/plans", createBody)
	if unknown.Code != 422 || !strings.Contains(unknown.Body.String(), "unknown field") {
		t.Fatalf("unknown JSON field accepted: %d %s", unknown.Code, unknown.Body.String())
	}
}

func TestRequestBodyLimit(t *testing.T) {
	repo, _ := store.Open(t.TempDir())
	defer repo.Close()
	auditor, _ := audit.Open(t.TempDir())
	defer auditor.Close()
	handler := httpapi.New(rigging.NewService(repo, auditor)).Handler()
	body := bytes.NewBufferString(`{"idempotencyKey":"large-body-key","venueName":"` + strings.Repeat("x", 2<<20) + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/plans", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 422 {
		t.Fatalf("large body status = %d", response.Code)
	}
}

func performJSON(handler http.Handler, method, path string, value any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(value)
	request := httptest.NewRequest(method, path, bytes.NewReader(b))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
