package rigging

import (
	"context"
	"fmt"
)

func (s *Service) Review(ctx context.Context, planID string, expected int64, r ReviewRequest, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "review.decided", r)
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if err := mutable(p); err != nil {
		return nil, err
	}
	if clean(r.Reviewer) == "" {
		return nil, Invalid("reviewer", "安全负责人不能为空")
	}
	if r.Decision != "approve" && r.Decision != "return" {
		return nil, Invalid("decision", "决定必须为 approve 或 return")
	}
	now := s.now()
	refreshIssues(p, now, false)
	if r.Decision == "approve" {
		if len(p.Points) == 0 {
			return nil, fmt.Errorf("%w: 至少登记一个吊点", ErrStateConflict)
		}
		if issues := openIssues(p); len(issues) > 0 {
			return nil, fmt.Errorf("%w: 仍有 %d 个未关闭风险项", ErrStateConflict, len(issues))
		}
		digest, err := FrozenDigest(p)
		if err != nil {
			return nil, err
		}
		p.Status = StatusApproved
		p.FrozenAt = &now
		p.FrozenDigest = digest
		p.FrozenPoints = append([]SuspensionPoint(nil), p.Points...)
		p.FrozenTests = append([]LoadTest(nil), p.Tests...)
	} else {
		p.Status = StatusReturned
	}
	p.Reviews = append(p.Reviews, ReviewDecision{Decision: r.Decision, Reviewer: clean(r.Reviewer), Note: clean(r.Note), At: now})
	return s.commit(ctx, p, expected, "review.decided", clean(r.Reviewer), key, "", r, map[string]any{"decision": r.Decision, "frozenDigest": p.FrozenDigest, "note": clean(r.Note)})
}

func (s *Service) IssueCredential(ctx context.Context, planID string, expected int64, issuedBy, key string) (*RigPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request := struct {
		IssuedBy string `json:"issuedBy"`
	}{clean(issuedBy)}
	p, receipt, err := s.loadCommand(ctx, planID, expected, key, "credential.issued", request)
	if receipt != nil {
		return s.idempotentResult(ctx, receipt)
	}
	if err != nil {
		return nil, err
	}
	if p.Status != StatusApproved || p.FrozenAt == nil || p.FrozenDigest == "" {
		return nil, fmt.Errorf("%w: 方案尚未批准冻结", ErrStateConflict)
	}
	if clean(issuedBy) == "" {
		return nil, Invalid("issuedBy", "签发人不能为空")
	}
	// Prepare the credential without persisting it so a domain commit failure
	// cannot leak a credential record, consume the global sequence or leave a
	// credential.sealed audit fact behind.
	credential, err := s.audit.Prepare(ctx, p.ID, p.FrozenDigest, clean(issuedBy))
	if err != nil {
		return nil, err
	}
	p.Credentials = append(p.Credentials, credential)
	actor := clean(issuedBy)
	payload := map[string]any{"credentialId": credential.ID, "sequence": credential.Sequence, "credentialDigest": credential.CredentialDigest}
	refreshTestConfiguration(p)
	p.Version = expected + 1
	receiptValue := CommandReceipt{PlanID: p.ID, Version: p.Version, ResourceID: credential.ID, Action: "credential.issued", RequestDigest: requestDigest(request)}
	if err := s.repo.Commit(ctx, p, expected, []DomainEvent{{Type: "credential.issued", Actor: actor, Payload: payload}}, key, "credential.issued", receiptValue); err != nil {
		return nil, err
	}
	// The domain accepted the credential: now durably seal it and append the
	// credential.sealed audit fact, followed by the credential.issued audit
	// mirror, preserving the audit timeline ordering.
	if _, err := s.audit.Seal(ctx, credential); err != nil {
		return nil, fmt.Errorf("seal credential: %w", err)
	}
	if s.audit != nil {
		if _, err := s.audit.Append(ctx, p.ID, "credential.issued", actor, payload); err != nil {
			return nil, fmt.Errorf("append audit: %w", err)
		}
	}
	return clonePlan(p), nil
}
