package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"stage-rig-clearance/internal/rigging"
)

type Store struct {
	mu                 sync.Mutex
	auditFile          *os.File
	credentialFile     *os.File
	records            []rigging.AuditRecord
	credentials        map[string]rigging.ClearanceCredential
	credentialSequence uint64
	now                func() time.Time
}

func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("audit directory is empty")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	s := &Store{credentials: make(map[string]rigging.ClearanceCredential), now: func() time.Time { return time.Now().UTC() }}
	auditPath := filepath.Join(dir, "audit.chain")
	credentialPath := filepath.Join(dir, "credentials.log")
	if err := s.loadAudit(auditPath); err != nil {
		return nil, err
	}
	if err := s.loadCredentials(credentialPath); err != nil {
		return nil, err
	}
	var err error
	s.auditFile, err = os.OpenFile(auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, err
	}
	s.credentialFile, err = os.OpenFile(credentialPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		_ = s.auditFile.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result error
	if s.auditFile != nil {
		result = s.auditFile.Close()
		s.auditFile = nil
	}
	if s.credentialFile != nil {
		if err := s.credentialFile.Close(); result == nil {
			result = err
		}
		s.credentialFile = nil
	}
	return result
}

func (s *Store) Append(_ context.Context, planID, action, actor string, payload map[string]any) (rigging.AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendLocked(planID, action, actor, payload)
}

func (s *Store) appendLocked(planID, action, actor string, payload map[string]any) (rigging.AuditRecord, error) {
	if s.auditFile == nil {
		return rigging.AuditRecord{}, errors.New("audit store is closed")
	}
	previous := ""
	if len(s.records) > 0 {
		previous = s.records[len(s.records)-1].Digest
	}
	record := rigging.AuditRecord{
		Sequence: uint64(len(s.records) + 1), PlanID: planID, Action: action, Actor: actor,
		At: s.now(), Payload: payload, PreviousDigest: previous,
	}
	digest, err := auditDigest(record)
	if err != nil {
		return rigging.AuditRecord{}, err
	}
	record.Digest = digest
	if err := writeFrame(s.auditFile, record); err != nil {
		return rigging.AuditRecord{}, err
	}
	if err := s.auditFile.Sync(); err != nil {
		return rigging.AuditRecord{}, err
	}
	s.records = append(s.records, record)
	return record, nil
}

func (s *Store) Issue(ctx context.Context, planID, frozenDigest, issuedBy string) (rigging.ClearanceCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, err := s.prepareLocked(planID, frozenDigest, issuedBy)
	if err != nil {
		return rigging.ClearanceCredential{}, err
	}
	if _, err := s.sealLocked(credential); err != nil {
		return rigging.ClearanceCredential{}, err
	}
	return credential, nil
}

// Prepare computes the next clearance credential, including its sequence and
// digest, without persisting anything. The caller can stage the domain commit
// first and only call Seal once the domain has accepted the credential, so a
// domain commit failure leaves no credential frame, no consumed sequence and
// no audit fact behind.
func (s *Store) Prepare(_ context.Context, planID, frozenDigest, issuedBy string) (rigging.ClearanceCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepareLocked(planID, frozenDigest, issuedBy)
}

// Seal durably appends a previously prepared credential and the matching
// credential.sealed audit fact. It verifies that the credential's sequence is
// still the next one to seal, which keeps the global sequence contiguous when
// the caller holds the issuing command lock between Prepare and Seal.
func (s *Store) Seal(_ context.Context, credential rigging.ClearanceCredential) (rigging.AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sealLocked(credential)
}

func (s *Store) prepareLocked(planID, frozenDigest, issuedBy string) (rigging.ClearanceCredential, error) {
	if s.credentialFile == nil {
		return rigging.ClearanceCredential{}, errors.New("audit store is closed")
	}
	credential := rigging.ClearanceCredential{
		ID: fmt.Sprintf("clearance-%06d", s.credentialSequence+1), PlanID: planID,
		Sequence: s.credentialSequence + 1, FrozenDigest: frozenDigest, IssuedBy: issuedBy, IssuedAt: s.now(),
	}
	digest, err := credentialDigest(credential)
	if err != nil {
		return rigging.ClearanceCredential{}, err
	}
	credential.CredentialDigest = digest
	return credential, nil
}

func (s *Store) sealLocked(credential rigging.ClearanceCredential) (rigging.AuditRecord, error) {
	if s.credentialFile == nil {
		return rigging.AuditRecord{}, errors.New("audit store is closed")
	}
	if credential.Sequence != s.credentialSequence+1 {
		return rigging.AuditRecord{}, fmt.Errorf("credential sequence stale: got %d want %d", credential.Sequence, s.credentialSequence+1)
	}
	digest, err := credentialDigest(credential)
	if err != nil {
		return rigging.AuditRecord{}, err
	}
	if digest != credential.CredentialDigest {
		return rigging.AuditRecord{}, fmt.Errorf("credential digest mismatch during seal")
	}
	if err := writeFrame(s.credentialFile, credential); err != nil {
		return rigging.AuditRecord{}, err
	}
	if err := s.credentialFile.Sync(); err != nil {
		return rigging.AuditRecord{}, err
	}
	s.credentialSequence = credential.Sequence
	s.credentials[credential.ID] = credential
	return s.appendLocked(credential.PlanID, "credential.sealed", credential.IssuedBy, map[string]any{"credentialId": credential.ID, "sequence": credential.Sequence, "frozenDigest": credential.FrozenDigest, "credentialDigest": digest})
}

func (s *Store) Timeline(_ context.Context, planID string) ([]rigging.AuditRecord, rigging.Verification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	verification := verifyRecords(s.records)
	result := make([]rigging.AuditRecord, 0)
	for _, record := range s.records {
		if record.PlanID == planID {
			result = append(result, record)
		}
	}
	return result, verification, nil
}

func (s *Store) VerifyCredential(_ context.Context, credential rigging.ClearanceCredential) (rigging.Verification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.credentials[credential.ID]
	if !ok {
		return rigging.Verification{Valid: false, Message: "凭据不在只追加签发日志中"}, nil
	}
	expected, err := credentialDigest(stored)
	if err != nil {
		return rigging.Verification{}, err
	}
	if expected != stored.CredentialDigest {
		return rigging.Verification{Valid: false, Message: "凭据摘要不匹配"}, nil
	}
	if stored != credential {
		return rigging.Verification{Valid: false, Message: "凭据内容与签发日志不一致"}, nil
	}
	if chain := verifyRecords(s.records); !chain.Valid {
		return chain, nil
	}
	return rigging.Verification{Valid: true, Message: "凭据摘要、冻结摘要引用和审计链均有效"}, nil
}

func verifyRecords(records []rigging.AuditRecord) rigging.Verification {
	previous := ""
	for i, record := range records {
		if record.Sequence != uint64(i+1) {
			return rigging.Verification{Valid: false, Message: fmt.Sprintf("审计序号不连续：位置 %d", i+1)}
		}
		if record.PreviousDigest != previous {
			return rigging.Verification{Valid: false, Message: fmt.Sprintf("审计前序摘要不匹配：序号 %d", record.Sequence)}
		}
		digest, err := auditDigest(record)
		if err != nil || digest != record.Digest {
			return rigging.Verification{Valid: false, Message: fmt.Sprintf("审计摘要不匹配：序号 %d", record.Sequence)}
		}
		previous = record.Digest
	}
	return rigging.Verification{Valid: true, Message: "审计链序号连续且摘要完整"}
}

func (s *Store) loadAudit(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	err = readFrames(file, func(payload []byte) error {
		var record rigging.AuditRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return err
		}
		s.records = append(s.records, record)
		return nil
	})
	if err != nil {
		return fmt.Errorf("load audit chain: %w", err)
	}
	if verification := verifyRecords(s.records); !verification.Valid {
		return errors.New(verification.Message)
	}
	return nil
}

func (s *Store) loadCredentials(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return readFrames(file, func(payload []byte) error {
		var credential rigging.ClearanceCredential
		if err := json.Unmarshal(payload, &credential); err != nil {
			return err
		}
		if credential.Sequence != s.credentialSequence+1 {
			return fmt.Errorf("credential sequence gap: got %d want %d", credential.Sequence, s.credentialSequence+1)
		}
		digest, err := credentialDigest(credential)
		if err != nil {
			return err
		}
		if digest != credential.CredentialDigest {
			return fmt.Errorf("credential %s digest mismatch", credential.ID)
		}
		if _, exists := s.credentials[credential.ID]; exists {
			return fmt.Errorf("duplicate credential %s", credential.ID)
		}
		s.credentials[credential.ID] = credential
		s.credentialSequence = credential.Sequence
		return nil
	})
}

var _ rigging.Auditor = (*Store)(nil)
