package rigging

import (
	"context"
	"time"
)

func (s *Service) CreatePlan(ctx context.Context, r CreatePlanRequest, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateCreate(r); err != nil {
		return nil, err
	}
	if clean(key) == "" {
		return nil, Invalid("idempotencyKey", "幂等键不能为空")
	}
	if receipt, err := s.repo.LookupCommand(ctx, key, "plan.created"); err == nil {
		if receipt.RequestDigest != "" && receipt.RequestDigest != requestDigest(r) {
			return nil, ErrIdempotency
		}
		return s.idempotentResult(ctx, receipt)
	}
	now := s.now()
	p := &RigPlan{
		ID: newID("plan"), VenueName: clean(r.VenueName), PerformanceDate: r.PerformanceDate,
		RatedTotalLoadKg: r.RatedTotalLoadKg, OwnerName: clean(r.OwnerName), Status: StatusDraft,
		Version: 1, CreatedAt: now, Points: []SuspensionPoint{}, Tests: []LoadTest{}, Issues: []SafetyIssue{},
	}
	receipt := CommandReceipt{PlanID: p.ID, Version: 1, ResourceID: p.ID, Action: "plan.created", RequestDigest: requestDigest(r)}
	payload := map[string]any{"venueName": p.VenueName, "performanceDate": p.PerformanceDate, "ratedTotalLoadKg": p.RatedTotalLoadKg}
	if err := s.repo.Commit(ctx, p, 0, []DomainEvent{{Type: "plan.created", Actor: p.OwnerName, Payload: payload}}, key, "plan.created", receipt); err != nil {
		return nil, err
	}
	if s.audit != nil {
		if _, err := s.audit.Append(ctx, p.ID, "plan.created", p.OwnerName, payload); err != nil {
			return nil, err
		}
	}
	return clonePlan(p), nil
}

func (s *Service) AddPoint(ctx context.Context, planID string, expected int64, r AddPointRequest, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "point.added", r)
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	if err := validatePoint(p, r); err != nil {
		return nil, err
	}
	point := SuspensionPoint{
		ID: newID("point"), PlanID: p.ID, Label: clean(r.Label), RatedLoadKg: r.RatedLoadKg,
		PlannedLoadKg: r.PlannedLoadKg, DeviceModel: clean(r.DeviceModel), CableSpec: clean(r.CableSpec),
		PrimaryPointID: clean(r.PrimaryPointID), RedundantPointID: clean(r.RedundantPointID), CertificateExpiresOn: r.CertificateExpiresOn,
	}
	point.ConfigRevision = 1
	point.ConfigDigest = PointConfigDigest(point)
	p.Points = append(p.Points, point)
	if err := validatePointGraph(p); err != nil {
		return nil, err
	}
	refreshIssues(p, s.now(), false)
	return s.commit(ctx, p, expected, "point.added", p.OwnerName, key, point.ID, r, map[string]any{"pointId": point.ID, "label": point.Label, "configRevision": point.ConfigRevision, "configDigest": point.ConfigDigest})
}

func futureDate(days int) string { return time.Now().UTC().AddDate(0, 0, days).Format("2006-01-02") }
