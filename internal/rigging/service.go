package rigging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Service struct {
	repo             Repository
	audit            Auditor
	now              func() time.Time
	mu               sync.Mutex
	timelineMu       sync.Mutex
	timelineCache    map[string]timelineCacheEntry
	timelineVersions map[string]uint64
}

type timelineCacheEntry struct {
	records      []AuditRecord
	verification Verification
}

func NewService(repo Repository, audit Auditor) *Service {
	return &Service{
		repo: repo, audit: audit, now: func() time.Time { return time.Now().UTC() },
		timelineCache:    make(map[string]timelineCacheEntry),
		timelineVersions: make(map[string]uint64),
	}
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
	s.invalidateTimeline(p.ID)
	return clonePlan(p), nil
}

// invalidateTimeline drops the cached audit timeline for a plan after any audit
// append succeeds. It must be called while the originating write still holds
// s.mu so that the generation bump is observed by Timeline reads that race with
// the append: a read that snapshotted a stale timeline before the append will
// either see the bumped generation (and reload) or have its own store rejected.
func (s *Service) invalidateTimeline(planID string) {
	s.timelineMu.Lock()
	defer s.timelineMu.Unlock()
	s.timelineVersions[planID]++
	delete(s.timelineCache, planID)
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
	// Capture the generation before reading anything. A write that appends to
	// the audit timeline bumps this generation, so any snapshot taken against
	// an older generation must not be allowed to repopulate the cache after a
	// concurrent append completes.
	generation := s.timelineGeneration(planID)
	if entry, ok := s.timelineEntry(planID, generation); ok {
		return cloneAuditRecords(entry.records), entry.verification, nil
	}
	records, verification, err := s.audit.Timeline(ctx, planID)
	if err != nil {
		return nil, Verification{}, err
	}
	// Only store the freshly loaded snapshot when no audit append for this plan
	// completed while the load was in flight. If the generation advanced, the
	// stale snapshot is discarded and the next reader will reload from the audit
	// store, which already reflects every committed append.
	s.storeTimelineEntry(planID, generation, timelineCacheEntry{records: cloneAuditRecords(records), verification: verification})
	return records, verification, nil
}

func (s *Service) timelineGeneration(planID string) uint64 {
	s.timelineMu.Lock()
	defer s.timelineMu.Unlock()
	return s.timelineVersions[planID]
}

func (s *Service) timelineEntry(planID string, generation uint64) (timelineCacheEntry, bool) {
	s.timelineMu.Lock()
	defer s.timelineMu.Unlock()
	entry, ok := s.timelineCache[planID]
	if !ok || s.timelineVersions[planID] != generation {
		return timelineCacheEntry{}, false
	}
	return entry, true
}

func (s *Service) storeTimelineEntry(planID string, generation uint64, entry timelineCacheEntry) {
	s.timelineMu.Lock()
	defer s.timelineMu.Unlock()
	if s.timelineVersions[planID] != generation {
		return
	}
	s.timelineCache[planID] = entry
}

func cloneAuditRecords(records []AuditRecord) []AuditRecord {
	b, _ := json.Marshal(records)
	var result []AuditRecord
	_ = json.Unmarshal(b, &result)
	return result
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
	p, credential, err := s.repo.FindCredential(ctx, credentialID)
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
