package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"stage-rig-clearance/internal/rigging"
)

type Repository struct {
	mu           sync.RWMutex
	dir          string
	logPath      string
	snapshotPath string
	log          *os.File
	sequence     uint64
	plans        map[string]*rigging.RigPlan
	commands     map[string]commandRecord
}

func Open(dir string) (*Repository, error) {
	if dir == "" {
		return nil, errors.New("store directory is empty")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	r := &Repository{
		dir: dir, logPath: filepath.Join(dir, "events.log"), snapshotPath: filepath.Join(dir, "projection.json"),
		plans: make(map[string]*rigging.RigPlan), commands: make(map[string]commandRecord),
	}
	if err := r.recover(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, err
	}
	r.log = file
	return r, nil
}

func (r *Repository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.log == nil {
		return nil
	}
	err := r.log.Close()
	r.log = nil
	return err
}

func (r *Repository) Get(_ context.Context, id string) (*rigging.RigPlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plans[id]
	if !ok {
		return nil, rigging.ErrNotFound
	}
	return copyPlan(p), nil
}

func (r *Repository) List(_ context.Context) ([]*rigging.RigPlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.plans))
	for id := range r.plans {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*rigging.RigPlan, 0, len(ids))
	for _, id := range ids {
		result = append(result, copyPlan(r.plans[id]))
	}
	return result, nil
}

func (r *Repository) FindCredential(_ context.Context, credentialID string) (*rigging.RigPlan, rigging.ClearanceCredential, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, plan := range r.plans {
		for _, credential := range plan.Credentials {
			if credential.ID == credentialID {
				return copyPlan(plan), credential, nil
			}
		}
	}
	return nil, rigging.ClearanceCredential{}, rigging.ErrNotFound
}

func (r *Repository) LookupCommand(_ context.Context, key, action string) (*rigging.CommandReceipt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.commands[key]
	if !ok {
		return nil, rigging.ErrNotFound
	}
	if record.Action != action {
		return nil, fmt.Errorf("%w: previous=%s requested=%s", rigging.ErrIdempotency, record.Action, action)
	}
	receipt := record.Receipt
	return &receipt, nil
}

func (r *Repository) Commit(_ context.Context, plan *rigging.RigPlan, expected int64, events []rigging.DomainEvent, key, action string, receipt rigging.CommandReceipt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validatePlanProjection(plan); err != nil {
		return fmt.Errorf("validate projection: %w", err)
	}
	if r.log == nil {
		return errors.New("repository is closed")
	}
	if existing, ok := r.commands[key]; ok {
		if existing.Action == action {
			return nil
		}
		return rigging.ErrIdempotency
	}
	current := int64(0)
	if p, ok := r.plans[plan.ID]; ok {
		current = p.Version
	}
	if current != expected {
		return fmt.Errorf("%w: current=%d expected=%d", rigging.ErrVersionConflict, current, expected)
	}
	if plan.Version != expected+1 {
		return fmt.Errorf("invalid new aggregate version %d", plan.Version)
	}
	frame := eventFrame{
		SchemaVersion: schemaVersion, Sequence: r.sequence + 1, PlanID: plan.ID,
		ExpectedVersion: expected, NewVersion: plan.Version, Events: events, Aggregate: copyPlan(plan),
		IdempotencyKey: key, Action: action, Receipt: receipt,
	}
	encoded, err := encodeFrame(frame)
	if err != nil {
		return err
	}
	if _, err := r.log.Write(encoded); err != nil {
		return err
	}
	if err := r.log.Sync(); err != nil {
		return err
	}
	r.sequence = frame.Sequence
	r.plans[plan.ID] = copyPlan(plan)
	r.commands[key] = commandRecord{Action: action, Receipt: receipt}
	return r.writeSnapshotLocked()
}

func copyPlan(plan *rigging.RigPlan) *rigging.RigPlan {
	b, _ := jsonMarshal(plan)
	var result rigging.RigPlan
	_ = jsonUnmarshal(b, &result)
	return &result
}
