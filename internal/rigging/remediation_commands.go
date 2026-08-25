package rigging

import "context"

func (s *Service) Remediate(ctx context.Context, planID string, expected int64, r RemediateRequest, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "issue.remediated", r)
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	issue, ok := findIssue(p, r.IssueID)
	if !ok {
		return nil, ErrNotFound
	}
	if issue.Status == IssueClosed {
		return nil, Invalid("issueId", "风险项已关闭")
	}
	if clean(r.Note) == "" || clean(r.RevisedBy) == "" {
		return nil, Invalid("note", "整改说明和整改人不能为空")
	}
	targetPointID := issue.PointID
	if targetPointID == "" {
		targetPointID = clean(r.PointID)
		if targetPointID == "" {
			return nil, Invalid("pointId", "方案级风险整改必须指定目标吊点")
		}
	}
	point, hasPoint := findPoint(p, targetPointID)
	if !hasPoint {
		return nil, Invalid("pointId", "整改目标吊点不存在")
	}
	before, after := map[string]any{}, map[string]any{}
	oldConfigDigest := point.ConfigDigest
	if hasPoint {
		before = map[string]any{"plannedLoadKg": point.PlannedLoadKg, "redundantPointId": point.RedundantPointID, "certificateExpiresOn": point.CertificateExpiresOn}
		if r.PlannedLoadKg != nil {
			if *r.PlannedLoadKg <= 0 {
				return nil, Invalid("plannedLoadKg", "计划载荷必须大于 0 kg")
			}
			point.PlannedLoadKg = *r.PlannedLoadKg
		}
		if r.RedundantPointID != nil {
			if *r.RedundantPointID == point.ID {
				return nil, Invalid("redundantPointId", "冗余吊点不能指向自身")
			}
			if _, exists := findPoint(p, *r.RedundantPointID); !exists {
				return nil, Invalid("redundantPointId", "冗余吊点不存在")
			}
			point.RedundantPointID = *r.RedundantPointID
		}
		if r.CertificateExpiresOn != nil {
			if _, e := parseDate(*r.CertificateExpiresOn); e != nil {
				return nil, Invalid("certificateExpiresOn", "证书日期必须为 YYYY-MM-DD")
			}
			point.CertificateExpiresOn = *r.CertificateExpiresOn
		}
		after = map[string]any{"plannedLoadKg": point.PlannedLoadKg, "redundantPointId": point.RedundantPointID, "certificateExpiresOn": point.CertificateExpiresOn}
	}
	if err := validatePointGraph(p); err != nil {
		return nil, err
	}
	if digest := PointConfigDigest(*point); digest != oldConfigDigest {
		point.ConfigRevision++
		if point.ConfigRevision < 2 {
			point.ConfigRevision = 2
		}
		point.ConfigDigest = digest
	}
	now := s.now()
	var replacedTaskIDs []string
	for i := range p.RetestTasks {
		task := &p.RetestTasks[i]
		if task.IssueID == issue.ID && task.Status == RetestTaskPending {
			task.Status = RetestTaskReplaced
			task.ReplacedAt = &now
			replacedTaskIDs = append(replacedTaskIDs, task.ID)
		}
	}
	issue.RemediationNote = clean(r.Note)
	issue.Status = IssueRetestPending
	revision := RemediationRevision{ID: newID("revision"), IssueID: issue.ID, PointID: targetPointID, Before: before, After: after, Note: clean(r.Note), RevisedBy: clean(r.RevisedBy), RevisedAt: now}
	p.Revisions = append(p.Revisions, revision)
	task := RetestTask{
		ID: newID("retest_task"), PlanID: p.ID, IssueID: issue.ID, PointID: targetPointID, RevisionID: revision.ID,
		PointConfigDigest: point.ConfigDigest, Status: RetestTaskPending, CreatedAt: now,
		Conditions:     RetestConditions{MinimumTargetLoadKg: point.PlannedLoadKg, MinimumHoldSeconds: 60, MaximumDeformationMm: 2, RequiredOutcome: OutcomePass},
		AttemptTestIDs: []string{},
	}
	p.RetestTasks = append(p.RetestTasks, task)
	p.Status = StatusRemediation
	return s.commit(ctx, p, expected, "issue.remediated", revision.RevisedBy, key, revision.ID, r, map[string]any{"issueId": issue.ID, "revisionId": revision.ID, "retestTaskId": task.ID, "conditions": task.Conditions, "replacedTaskIds": replacedTaskIDs, "before": before, "after": after})
}
