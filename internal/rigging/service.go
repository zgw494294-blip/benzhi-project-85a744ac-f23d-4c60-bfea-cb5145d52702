package rigging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Service struct {
	repo  Repository
	audit Auditor
	now   func() time.Time
	mu    sync.Mutex

	credentialCacheMu    sync.RWMutex
	credentialCacheID    string
	credentialCachePlan  *RigPlan
	credentialCacheValue ClearanceCredential
}

func NewService(repo Repository, audit Auditor) *Service {
	return &Service{repo: repo, audit: audit, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) GetPlan(ctx context.Context, id string) (*RigPlan, error) {
	return s.repo.Get(ctx, id)
}
func (s *Service) ListPlans(ctx context.Context) ([]*RigPlan, error) { return s.repo.List(ctx) }

func (s *Service) loadCommand(ctx context.Context, planID string, expected int64, key, action string, request any) (*RigPlan, *CommandReceipt, error) {
	if clean(key) == "" {
		return nil, nil, Invalid("idempotencyKey", "幂等键不能为空")
	}
	if receipt, err := s.repo.LookupCommand(ctx, key, action); err == nil {
		if receipt.PlanID != planID {
			return nil, nil, ErrIdempotency
		}
		if receipt.RequestDigest != "" && receipt.RequestDigest != requestDigest(request) {
			return nil, nil, ErrIdempotency
		}
		return nil, receipt, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, nil, err
	}
	p, err := s.repo.Get(ctx, planID)
	if err != nil {
		return nil, nil, err
	}
	if p.Version != expected {
		return nil, nil, fmt.Errorf("%w: current=%d expected=%d", ErrVersionConflict, p.Version, expected)
	}
	return clonePlan(p), nil, nil
}

func (s *Service) commit(ctx context.Context, p *RigPlan, expected int64, action, actor, key, resourceID string, request any, payload map[string]any) (*RigPlan, error) {
	refreshTestConfiguration(p)
	p.Version = expected + 1
	receipt := CommandReceipt{PlanID: p.ID, Version: p.Version, ResourceID: resourceID, Action: action, RequestDigest: requestDigest(request)}
	event := DomainEvent{Type: action, Actor: actor, Payload: payload}
	if err := s.repo.Commit(ctx, p, expected, []DomainEvent{event}, key, action, receipt); err != nil {
		return nil, err
	}
	if s.audit != nil {
		if _, err := s.audit.Append(ctx, p.ID, action, actor, payload); err != nil {
			return nil, fmt.Errorf("append audit: %w", err)
		}
	}
	return clonePlan(p), nil
}

func (s *Service) idempotentResult(ctx context.Context, receipt *CommandReceipt) (*RigPlan, error) {
	if receipt == nil {
		return nil, ErrNotFound
	}
	return s.repo.Get(ctx, receipt.PlanID)
}

func mutable(p *RigPlan) error {
	if p.Status == StatusApproved || p.FrozenAt != nil {
		return ErrFrozen
	}
	return nil
}

func (s *Service) Timeline(ctx context.Context, planID string) ([]AuditRecord, Verification, error) {
	if _, err := s.repo.Get(ctx, planID); err != nil {
		return nil, Verification{}, err
	}
	return s.audit.Timeline(ctx, planID)
}

func (s *Service) VerifyCredential(ctx context.Context, planID, credentialID string) (ClearanceCredential, Verification, error) {
	p, err := s.repo.Get(ctx, planID)
	if err != nil {
		return ClearanceCredential{}, Verification{}, err
	}
	for _, credential := range p.Credentials {
		if credential.ID == credentialID {
			v, err := s.audit.VerifyCredential(ctx, credential)
			if err == nil && credential.FrozenDigest != p.FrozenDigest {
				v = Verification{Valid: false, Message: "凭据冻结摘要与当前冻结投影不一致"}
			}
			return credential, v, err
		}
	}
	return ClearanceCredential{}, Verification{}, ErrNotFound
}

func (s *Service) VerifyCredentialGlobally(ctx context.Context, credentialID, digest string) (GlobalCredentialVerification, error) {
	credentialID = clean(credentialID)
	digest = clean(digest)
	if credentialID == "" {
		return GlobalCredentialVerification{}, Invalid("credentialId", "凭据标识不能为空")
	}
	if len(digest) != 64 {
		return GlobalCredentialVerification{}, Invalid("digest", "凭据摘要必须为完整的 64 位十六进制值")
	}
	for _, r := range digest {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return GlobalCredentialVerification{}, Invalid("digest", "凭据摘要必须为完整的 64 位十六进制值")
		}
	}
	p, credential, err := s.findCredential(ctx, credentialID)
	if err != nil {
		return GlobalCredentialVerification{}, err
	}
	result := GlobalCredentialVerification{
		CredentialID: credential.ID, Sequence: credential.Sequence, IssuedAt: credential.IssuedAt,
		IssuedBy: credential.IssuedBy, VenueName: p.VenueName, PerformanceDate: p.PerformanceDate,
	}
	result.Checks.CredentialDigest = strings.EqualFold(digest, credential.CredentialDigest)
	projectionDigest, digestErr := FrozenProjectionDigest(p)
	result.Checks.FrozenManifest = digestErr == nil && projectionDigest == p.FrozenDigest
	result.Checks.FrozenReference = credential.FrozenDigest == p.FrozenDigest
	result.Checks.Sequence = credential.Sequence > 0
	auditVerification, auditErr := s.audit.VerifyCredential(ctx, credential)
	timeline, chainVerification, timelineErr := s.audit.Timeline(ctx, p.ID)
	sealed := false
	for _, record := range timeline {
		if record.Action == "credential.sealed" && record.Payload["credentialId"] == credential.ID {
			sealed = true
			break
		}
	}
	result.Checks.AuditChain = auditErr == nil && timelineErr == nil && auditVerification.Valid && chainVerification.Valid && sealed
	result.Valid = result.Checks.CredentialDigest && result.Checks.FrozenManifest && result.Checks.FrozenReference && result.Checks.Sequence && result.Checks.AuditChain
	if result.Valid {
		result.Message = "凭据、冻结清单与审计链一致"
	} else if !result.Checks.CredentialDigest {
		result.Message = "输入的凭据摘要不匹配"
	} else {
		result.Message = "凭据存在，但完整性核验未通过"
	}
	return result, nil
}

func (s *Service) findCredential(ctx context.Context, credentialID string) (*RigPlan, ClearanceCredential, error) {
	s.credentialCacheMu.RLock()
	if s.credentialCacheID == credentialID && s.credentialCachePlan != nil {
		plan, credential := clonePlan(s.credentialCachePlan), s.credentialCacheValue
		s.credentialCacheMu.RUnlock()
		return plan, credential, nil
	}
	s.credentialCacheMu.RUnlock()

	p, credential, err := s.repo.FindCredential(ctx, credentialID)
	if err != nil {
		return nil, ClearanceCredential{}, err
	}
	planSnapshot, valueSnapshot := clonePlan(p), credential
	s.credentialCacheMu.Lock()
	s.credentialCacheID = credentialID
	s.credentialCachePlan = planSnapshot
	s.credentialCacheValue = valueSnapshot
	s.credentialCacheMu.Unlock()
	return p, credential, nil
}
