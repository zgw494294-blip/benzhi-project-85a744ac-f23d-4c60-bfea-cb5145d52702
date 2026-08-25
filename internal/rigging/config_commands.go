package rigging

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) UpdatePlan(ctx context.Context, planID string, expected int64, r UpdatePlanRequest, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateCreate(CreatePlanRequest(r)); err != nil {
		return nil, err
	}
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "plan.updated", r)
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	before := map[string]any{"venueName": p.VenueName, "performanceDate": p.PerformanceDate, "ratedTotalLoadKg": p.RatedTotalLoadKg, "ownerName": p.OwnerName}
	after := map[string]any{"venueName": clean(r.VenueName), "performanceDate": r.PerformanceDate, "ratedTotalLoadKg": r.RatedTotalLoadKg, "ownerName": clean(r.OwnerName)}
	if requestDigest(before) == requestDigest(after) {
		return nil, Invalid("body", "方案基础信息没有任何实际变化")
	}
	actor := p.OwnerName
	p.VenueName, p.PerformanceDate, p.RatedTotalLoadKg, p.OwnerName = clean(r.VenueName), r.PerformanceDate, r.RatedTotalLoadKg, clean(r.OwnerName)
	now := s.now()
	revision := PlanRevision{ID: newID("plan_revision"), Before: before, After: after, RevisedBy: actor, RevisedAt: now}
	p.PlanRevisions = append(p.PlanRevisions, revision)
	refreshIssues(p, now, false)
	if len(openIssues(p)) > 0 {
		p.Status = StatusRemediation
	}
	return s.commit(ctx, p, expected, "plan.updated", actor, key, revision.ID, r, map[string]any{"revisionId": revision.ID, "before": before, "after": after})
}

func (s *Service) UpdatePoint(ctx context.Context, planID, pointID string, expected int64, r UpdatePointRequest, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "point.updated", struct {
		PointID string             `json:"pointId"`
		Request UpdatePointRequest `json:"request"`
	}{pointID, r})
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	point, ok := findPoint(p, pointID)
	if !ok {
		return nil, ErrNotFound
	}
	if err := validatePointUpdate(p, pointID, r); err != nil {
		return nil, err
	}
	before := pointValues(*point)
	point.Label, point.RatedLoadKg, point.PlannedLoadKg = clean(r.Label), r.RatedLoadKg, r.PlannedLoadKg
	point.DeviceModel, point.CableSpec = clean(r.DeviceModel), clean(r.CableSpec)
	point.PrimaryPointID, point.RedundantPointID, point.CertificateExpiresOn = clean(r.PrimaryPointID), clean(r.RedundantPointID), r.CertificateExpiresOn
	after := pointValues(*point)
	if requestDigest(before) == requestDigest(after) {
		return nil, Invalid("body", "吊点配置没有任何实际变化")
	}
	if err := validatePointGraph(p); err != nil {
		return nil, err
	}
	point.ConfigRevision++
	if point.ConfigRevision < 2 {
		point.ConfigRevision = 2
	}
	point.ConfigDigest = PointConfigDigest(*point)
	now := s.now()
	refreshIssues(p, now, false)
	if len(openIssues(p)) > 0 {
		p.Status = StatusRemediation
	}
	request := struct {
		PointID string             `json:"pointId"`
		Request UpdatePointRequest `json:"request"`
	}{pointID, r}
	return s.commit(ctx, p, expected, "point.updated", p.OwnerName, key, point.ID, request, map[string]any{"pointId": point.ID, "before": before, "after": after, "configRevision": point.ConfigRevision, "configDigest": point.ConfigDigest})
}

func (s *Service) RemovePoint(ctx context.Context, planID, pointID string, expected int64, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request := struct {
		PointID string `json:"pointId"`
	}{pointID}
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "point.removed", request)
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	point, ok := findPoint(p, pointID)
	if !ok {
		return nil, ErrNotFound
	}
	check := pointRemovalCheck(p, pointID)
	blockers := append(append([]string{}, check.References...), check.History...)
	if len(blockers) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrStateConflict, strings.Join(blockers, "；"))
	}
	removed := *point
	points := p.Points[:0]
	for _, candidate := range p.Points {
		if candidate.ID != pointID {
			points = append(points, candidate)
		}
	}
	p.Points = points
	issues := p.Issues[:0]
	for _, issue := range p.Issues {
		if issue.PointID != pointID {
			issues = append(issues, issue)
		}
	}
	p.Issues = issues
	refreshIssues(p, s.now(), false)
	return s.commit(ctx, p, expected, "point.removed", p.OwnerName, key, pointID, request, map[string]any{"pointId": pointID, "removed": pointValues(removed)})
}

func (s *Service) CheckPointRemoval(ctx context.Context, planID, pointID string) (PointRemovalCheck, error) {
	p, err := s.repo.Get(ctx, planID)
	if err != nil {
		return PointRemovalCheck{}, err
	}
	if _, ok := findPoint(p, pointID); !ok {
		return PointRemovalCheck{}, ErrNotFound
	}
	check := pointRemovalCheck(p, pointID)
	if err := mutable(p); err != nil {
		check.Allowed = false
		check.Message = "方案已冻结，不能移除吊点"
	}
	return check, nil
}

func pointRemovalCheck(p *RigPlan, pointID string) PointRemovalCheck {
	check := PointRemovalCheck{References: []string{}, History: []string{}}
	for _, other := range p.Points {
		if other.ID != pointID && (other.PrimaryPointID == pointID || other.RedundantPointID == pointID) {
			check.References = append(check.References, fmt.Sprintf("被吊点 %s(%s) 引用", other.Label, other.ID))
		}
	}
	for _, test := range p.Tests {
		if test.PointID == pointID {
			check.History = append(check.History, "存在不可删除的试验历史")
			break
		}
	}
	for _, revision := range p.Revisions {
		if revision.PointID == pointID {
			check.History = append(check.History, "存在不可删除的整改修订历史")
			break
		}
	}
	check.Allowed = len(check.References) == 0 && len(check.History) == 0
	if check.Allowed {
		check.Message = "未发现引用、试验或整改历史，可以移除"
	} else {
		check.Message = strings.Join(append(append([]string{}, check.References...), check.History...), "；")
	}
	return check
}

func pointValues(point SuspensionPoint) map[string]any {
	return map[string]any{"label": point.Label, "ratedLoadKg": point.RatedLoadKg, "plannedLoadKg": point.PlannedLoadKg, "deviceModel": point.DeviceModel, "cableSpec": point.CableSpec, "primaryPointId": point.PrimaryPointID, "redundantPointId": point.RedundantPointID, "certificateExpiresOn": point.CertificateExpiresOn}
}
