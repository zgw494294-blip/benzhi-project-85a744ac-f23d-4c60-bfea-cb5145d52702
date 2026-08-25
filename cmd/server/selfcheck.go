package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"stage-rig-clearance/internal/rigging"
)

type selfcheckClient struct {
	base string
	http *http.Client
	step int
}

func runSelfcheck(app *application, listener net.Listener) error {
	serveDone := make(chan error, 1)
	go func() { serveDone <- app.httpServer.Serve(listener) }()
	client := &selfcheckClient{base: "http://" + listener.Addr().String(), http: &http.Client{Timeout: 3 * time.Second}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	flowErr := client.fullFlow(ctx)
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	shutdownErr := app.httpServer.Shutdown(shutdownContext)
	select {
	case <-serveDone:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("selfcheck server did not stop")
	}
	if flowErr != nil {
		return flowErr
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	fmt.Println("selfcheck 通过：真实 HTTP 主链路已完成，凭据与审计链核验有效")
	return nil
}

func (c *selfcheckClient) fullFlow(ctx context.Context) error {
	showDate := time.Now().UTC().AddDate(0, 2, 0).Format("2006-01-02")
	certDate := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	var plan rigging.RigPlan
	if err := c.post(ctx, "/api/plans", map[string]any{"idempotencyKey": c.key("create"), "venueName": "自检剧场", "performanceDate": showDate, "ratedTotalLoadKg": 2000, "ownerName": "机械技师"}, &plan); err != nil {
		return err
	}
	if err := c.post(ctx, "/api/plans/"+plan.ID+"/points", map[string]any{"idempotencyKey": c.key("point-a"), "expectedVersion": plan.Version, "label": "A1", "ratedLoadKg": 800, "plannedLoadKg": 500, "deviceModel": "HOIST-X", "cableSpec": "8mm-6x19", "primaryPointId": "", "redundantPointId": "", "certificateExpiresOn": certDate}, &plan); err != nil {
		return err
	}
	pointA := plan.Points[0].ID
	if err := c.post(ctx, "/api/plans/"+plan.ID+"/points", map[string]any{"idempotencyKey": c.key("point-b"), "expectedVersion": plan.Version, "label": "B1", "ratedLoadKg": 800, "plannedLoadKg": 500, "deviceModel": "HOIST-X", "cableSpec": "8mm-6x19", "primaryPointId": pointA, "redundantPointId": "", "certificateExpiresOn": certDate}, &plan); err != nil {
		return err
	}
	pointB := plan.Points[1].ID
	batch := []map[string]any{
		{"pointId": pointA, "pointConfigDigest": plan.Points[0].ConfigDigest, "testKind": "initial", "targetLoadKg": 625, "measuredLoadKg": 630, "holdSeconds": 90, "deformationMm": 0.4, "outcome": "pass", "evidenceDigest": "sha256:selfcheck-initial-a", "performedBy": "载荷试验员"},
		{"pointId": pointB, "pointConfigDigest": plan.Points[1].ConfigDigest, "testKind": "initial", "targetLoadKg": 625, "measuredLoadKg": 630, "holdSeconds": 90, "deformationMm": 0.4, "outcome": "pass", "evidenceDigest": "sha256:selfcheck-initial-b", "performedBy": "载荷试验员"},
	}
	var batchResult struct {
		Plan rigging.RigPlan `json:"plan"`
	}
	if err := c.post(ctx, "/api/plans/"+plan.ID+"/tests/batch", map[string]any{"idempotencyKey": c.key("initial-batch"), "expectedVersion": plan.Version, "tests": batch}, &batchResult); err != nil {
		return err
	}
	plan = batchResult.Plan
	issueID := ""
	for _, issue := range plan.Issues {
		if issue.PointID == pointA && issue.RuleCode == "REDUNDANCY_MISSING" && issue.Status != rigging.IssueClosed {
			issueID = issue.ID
		}
	}
	if issueID == "" {
		return fmt.Errorf("selfcheck expected redundancy issue")
	}
	if err := c.post(ctx, "/api/plans/"+plan.ID+"/remediations", map[string]any{"idempotencyKey": c.key("remediate"), "expectedVersion": plan.Version, "issueId": issueID, "note": "绑定独立 B1 冗余吊点", "revisedBy": "机械技师", "redundantPointId": pointB}, &plan); err != nil {
		return err
	}
	if err := c.test(ctx, &plan, pointA, rigging.TestRetest, "retest-a"); err != nil {
		return err
	}
	if err := c.post(ctx, "/api/plans/"+plan.ID+"/review", map[string]any{"idempotencyKey": c.key("review"), "expectedVersion": plan.Version, "decision": "approve", "reviewer": "安全负责人", "note": "配置与试验完整"}, &plan); err != nil {
		return err
	}
	if plan.Status != rigging.StatusApproved || plan.FrozenDigest == "" {
		return fmt.Errorf("selfcheck plan was not frozen")
	}
	if err := c.post(ctx, "/api/plans/"+plan.ID+"/credentials", map[string]any{"idempotencyKey": c.key("issue"), "expectedVersion": plan.Version, "issuedBy": "安全负责人"}, &plan); err != nil {
		return err
	}
	if len(plan.Credentials) != 1 {
		return fmt.Errorf("selfcheck expected one credential")
	}
	var verification struct {
		Verification rigging.Verification `json:"verification"`
	}
	if err := c.get(ctx, "/api/plans/"+plan.ID+"/credentials/"+plan.Credentials[0].ID+"/verify", &verification); err != nil {
		return err
	}
	if !verification.Verification.Valid {
		return fmt.Errorf("selfcheck credential invalid: %s", verification.Verification.Message)
	}
	if err := c.get(ctx, "/api/credentials/"+plan.Credentials[0].ID+"/verify?digest="+plan.Credentials[0].CredentialDigest, &verification); err != nil {
		return err
	}
	if !verification.Verification.Valid {
		return fmt.Errorf("selfcheck global credential invalid: %s", verification.Verification.Message)
	}
	var timeline struct {
		Verification rigging.Verification  `json:"verification"`
		Records      []rigging.AuditRecord `json:"records"`
	}
	if err := c.get(ctx, "/api/plans/"+plan.ID+"/audit", &timeline); err != nil {
		return err
	}
	if !timeline.Verification.Valid || len(timeline.Records) < 9 {
		return fmt.Errorf("selfcheck audit timeline invalid or incomplete")
	}
	return nil
}

func (c *selfcheckClient) test(ctx context.Context, plan *rigging.RigPlan, pointID string, kind rigging.TestKind, label string) error {
	body := map[string]any{"idempotencyKey": c.key(label), "expectedVersion": plan.Version, "pointId": pointID, "testKind": kind, "targetLoadKg": 625, "measuredLoadKg": 630, "holdSeconds": 90, "deformationMm": 0.4, "outcome": "pass", "evidenceDigest": "sha256:selfcheck-" + label, "performedBy": "载荷试验员"}
	if kind == rigging.TestRetest {
		for _, task := range plan.RetestTasks {
			if task.PointID == pointID && task.Status == rigging.RetestTaskPending {
				body["retestTaskId"] = task.ID
				break
			}
		}
	}
	return c.post(ctx, "/api/plans/"+plan.ID+"/tests", body, plan)
}

func (c *selfcheckClient) key(label string) string {
	c.step++
	return fmt.Sprintf("selfcheck-%02d-%s", c.step, label)
}

func (c *selfcheckClient) post(ctx context.Context, path string, body any, destination any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, destination)
}

func (c *selfcheckClient) get(ctx context.Context, path string, destination any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, destination)
}

func (c *selfcheckClient) do(req *http.Request, destination any) error {
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned %d: %s", req.Method, req.URL.Path, response.StatusCode, string(b))
	}
	if err := json.Unmarshal(b, destination); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.Path, err)
	}
	return nil
}
