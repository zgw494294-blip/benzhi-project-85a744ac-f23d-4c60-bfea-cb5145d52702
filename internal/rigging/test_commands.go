package rigging

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const maxTestBatchSize = 50

func (s *Service) RecordTest(ctx context.Context, planID string, expected int64, r RecordTestRequest, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	action := "test.recorded"
	if r.TestKind == TestRetest {
		action = "test.retested"
	}
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, action, r)
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	if err := validateTest(p, r); err != nil {
		return nil, err
	}
	point, _ := findPoint(p, r.PointID)
	if r.PointConfigDigest != "" && r.PointConfigDigest != point.ConfigDigest {
		return nil, fmt.Errorf("%w: 吊点配置摘要已变化", ErrStateConflict)
	}
	var task *RetestTask
	if r.TestKind == TestRetest {
		task, err = resolveRetestTask(p, r)
		if err != nil {
			return nil, err
		}
	}
	now := s.now()
	if task != nil && !now.After(task.CreatedAt) {
		return nil, fmt.Errorf("%w: 复测时间必须晚于整改任务创建时间", ErrStateConflict)
	}
	test := newLoadTest(p, *point, r, now)
	if task != nil {
		test.RetestTaskID = task.ID
	}
	p.Tests = append(p.Tests, test)
	if p.Status == StatusDraft {
		p.Status = StatusTesting
	}
	if task == nil {
		refreshIssues(p, now, false)
	} else {
		task.AttemptTestIDs = append(task.AttemptTestIDs, test.ID)
		task.UnmetConditions = unmetRetestConditions(test, task.Conditions)
		refreshIssues(p, now, false)
		if len(task.UnmetConditions) == 0 && !ruleStillActive(p, task.IssueID) {
			task.Status = RetestTaskClosed
			task.ClosedAt = &now
			task.ClosureBasis = fmt.Sprintf("复测 %s 满足任务条件，且绑定规则已不再命中", test.ID)
			if issue, ok := findIssue(p, task.IssueID); ok {
				issue.Status = IssueClosed
				issue.ClosedAt = &now
			}
		}
	}
	if len(openIssues(p)) > 0 {
		p.Status = StatusRemediation
	} else {
		p.Status = StatusTesting
	}
	payload := map[string]any{
		"testId": test.ID, "pointId": test.PointID, "kind": test.TestKind, "outcome": test.Outcome, "retestTaskId": test.RetestTaskID,
	}
	if task != nil {
		payload["unmetConditions"] = task.UnmetConditions
		payload["taskStatus"] = task.Status
		payload["closureBasis"] = task.ClosureBasis
	}
	return s.commit(ctx, p, expected, action, test.PerformedBy, key, test.ID, r, payload)
}

func (s *Service) RecordTestBatch(ctx context.Context, planID string, expected int64, r RecordTestBatchRequest, key string) (*TestBatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "tests.batch_recorded", r)
	if receipt != nil {
		plan, loadErr := s.idempotentResult(ctx, receipt)
		if loadErr != nil {
			return nil, loadErr
		}
		return batchResult(plan, receipt.BatchID, receipt.ResourceIDs), nil
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	if len(r.Tests) == 0 || len(r.Tests) > maxTestBatchSize {
		return nil, Invalid("tests", fmt.Sprintf("批量试验条数必须为 1-%d 条", maxTestBatchSize))
	}
	var fields []FieldError
	seen := map[string]bool{}
	for index, row := range r.Tests {
		prefix := fmt.Sprintf("tests[%d].", index)
		if seen[row.PointID] {
			fields = append(fields, FieldError{prefix + "pointId", "同一批次不能重复吊点"})
		}
		seen[row.PointID] = true
		if row.TestKind != TestInitial {
			fields = append(fields, FieldError{prefix + "testKind", "批量归档仅接受 initial 初次试验"})
		}
		if err := validateTest(p, row); err != nil {
			var validation *ValidationError
			if errors.As(err, &validation) {
				for _, field := range validation.Fields {
					fields = append(fields, FieldError{prefix + field.Field, field.Message})
				}
			}
		}
		if point, ok := findPoint(p, row.PointID); ok {
			if clean(row.PointConfigDigest) == "" {
				fields = append(fields, FieldError{prefix + "pointConfigDigest", "必须提供当前完整配置摘要"})
			} else if row.PointConfigDigest != point.ConfigDigest {
				fields = append(fields, FieldError{prefix + "pointConfigDigest", "吊点配置摘要与当前配置不一致"})
			}
		}
	}
	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}
	now := s.now()
	batchID := newID("test_batch")
	ids := make([]string, 0, len(r.Tests))
	for _, row := range r.Tests {
		point, _ := findPoint(p, row.PointID)
		test := newLoadTest(p, *point, row, now)
		p.Tests = append(p.Tests, test)
		ids = append(ids, test.ID)
	}
	refreshIssues(p, now, false)
	if len(openIssues(p)) > 0 {
		p.Status = StatusRemediation
	} else {
		p.Status = StatusTesting
	}
	refreshTestConfiguration(p)
	p.Version = expected + 1
	receiptValue := CommandReceipt{PlanID: p.ID, Version: p.Version, ResourceID: batchID, ResourceIDs: ids, BatchID: batchID, Action: "tests.batch_recorded", RequestDigest: requestDigest(r)}
	payload := map[string]any{"batchId": batchID, "pointCount": len(ids), "testIds": ids, "result": "archived"}
	if err := s.repo.Commit(ctx, p, expected, []DomainEvent{{Type: "tests.batch_recorded", Actor: r.Tests[0].PerformedBy, Payload: payload}}, key, "tests.batch_recorded", receiptValue); err != nil {
		return nil, err
	}
	if s.audit != nil {
		if _, err := s.audit.Append(ctx, p.ID, "tests.batch_recorded", r.Tests[0].PerformedBy, payload); err != nil {
			return nil, fmt.Errorf("append audit: %w", err)
		}
	}
	return batchResult(p, batchID, ids), nil
}

func newLoadTest(p *RigPlan, point SuspensionPoint, r RecordTestRequest, now time.Time) LoadTest {
	return LoadTest{
		ID: newID("test"), PlanID: p.ID, PointID: r.PointID, TestKind: r.TestKind,
		TargetLoadKg: r.TargetLoadKg, MeasuredLoadKg: r.MeasuredLoadKg, HoldSeconds: r.HoldSeconds,
		DeformationMm: r.DeformationMm, Outcome: r.Outcome, EvidenceDigest: clean(r.EvidenceDigest),
		PerformedBy: clean(r.PerformedBy), PerformedAt: now, PointConfigRevision: point.ConfigRevision,
		PointConfigDigest: point.ConfigDigest, CurrentConfiguration: true,
	}
}

func resolveRetestTask(p *RigPlan, r RecordTestRequest) (*RetestTask, error) {
	if clean(r.RetestTaskID) != "" {
		for i := range p.RetestTasks {
			task := &p.RetestTasks[i]
			if task.ID != r.RetestTaskID {
				continue
			}
			if task.Status != RetestTaskPending {
				return nil, fmt.Errorf("%w: 复测任务不是待处理状态", ErrStateConflict)
			}
			if task.PointID != r.PointID {
				return nil, fmt.Errorf("%w: 复测任务绑定了其他吊点", ErrStateConflict)
			}
			point, ok := findPoint(p, r.PointID)
			if !ok || point.ConfigDigest != task.PointConfigDigest {
				return nil, fmt.Errorf("%w: 整改后吊点配置摘要已再次变化", ErrStateConflict)
			}
			return task, nil
		}
		return nil, ErrNotFound
	}
	var only *RetestTask
	for i := range p.RetestTasks {
		if p.RetestTasks[i].PointID == r.PointID && p.RetestTasks[i].Status == RetestTaskPending {
			if only != nil {
				return nil, Invalid("retestTaskId", "存在多个待复测任务，必须明确指定")
			}
			only = &p.RetestTasks[i]
		}
	}
	if only == nil {
		return nil, Invalid("retestTaskId", "该吊点没有待处理复测任务")
	}
	if point, ok := findPoint(p, r.PointID); !ok || point.ConfigDigest != only.PointConfigDigest {
		return nil, fmt.Errorf("%w: 整改后吊点配置摘要已再次变化", ErrStateConflict)
	}
	return only, nil
}

func unmetRetestConditions(test LoadTest, conditions RetestConditions) []string {
	var result []string
	if test.TargetLoadKg < conditions.MinimumTargetLoadKg || test.MeasuredLoadKg < test.TargetLoadKg {
		result = append(result, "目标或实测载荷未达到最低条件")
	}
	if test.HoldSeconds < conditions.MinimumHoldSeconds {
		result = append(result, fmt.Sprintf("保持时长不足 %d 秒", conditions.MinimumHoldSeconds))
	}
	if test.DeformationMm > conditions.MaximumDeformationMm {
		result = append(result, fmt.Sprintf("变形量超过 %.2f mm", conditions.MaximumDeformationMm))
	}
	if test.Outcome != conditions.RequiredOutcome {
		result = append(result, "试验结论未通过")
	}
	return result
}

func ruleStillActive(p *RigPlan, issueID string) bool {
	issue, ok := findIssue(p, issueID)
	if !ok {
		return true
	}
	for _, finding := range evaluateRules(p) {
		if finding.PointID == issue.PointID && finding.Code == issue.RuleCode {
			return true
		}
	}
	return false
}

func batchResult(plan *RigPlan, batchID string, ids []string) *TestBatchResult {
	result := &TestBatchResult{BatchID: batchID, Plan: clonePlan(plan)}
	for _, id := range ids {
		for _, test := range plan.Tests {
			if test.ID != id {
				continue
			}
			var findings []string
			for _, finding := range evaluateRules(plan) {
				if finding.PointID == test.PointID {
					findings = append(findings, finding.Code)
				}
			}
			result.Tests = append(result.Tests, ArchivedTestResult{TestID: id, PointID: test.PointID, Findings: findings})
		}
	}
	return result
}
