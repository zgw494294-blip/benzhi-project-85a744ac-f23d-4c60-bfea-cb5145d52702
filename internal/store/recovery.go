package store

import (
	"fmt"
	"os"

	"stage-rig-clearance/internal/rigging"
)

func (r *Repository) recover() error {
	file, err := os.Open(r.logPath)
	if os.IsNotExist(err) {
		if err := validateSnapshot(r.snapshotPath, 0); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	var sequence uint64
	err = decodeFrames(file, func(frame eventFrame) error {
		if frame.Sequence != sequence+1 {
			return fmt.Errorf("event sequence gap: got %d want %d", frame.Sequence, sequence+1)
		}
		if frame.Aggregate == nil || frame.Aggregate.ID != frame.PlanID {
			return fmt.Errorf("event frame %d has invalid aggregate", frame.Sequence)
		}
		current := int64(0)
		if plan, ok := r.plans[frame.PlanID]; ok {
			current = plan.Version
		}
		if frame.ExpectedVersion != current || frame.NewVersion != current+1 || frame.Aggregate.Version != frame.NewVersion {
			return fmt.Errorf("event frame %d aggregate version discontinuity", frame.Sequence)
		}
		if frame.IdempotencyKey == "" {
			return fmt.Errorf("event frame %d has empty idempotency key", frame.Sequence)
		}
		if _, exists := r.commands[frame.IdempotencyKey]; exists {
			return fmt.Errorf("event frame %d repeats idempotency key", frame.Sequence)
		}
		r.plans[frame.PlanID] = copyPlan(frame.Aggregate)
		r.commands[frame.IdempotencyKey] = commandRecord{Action: frame.Action, Receipt: frame.Receipt}
		sequence = frame.Sequence
		return nil
	})
	if err != nil {
		return fmt.Errorf("recover event log: %w", err)
	}
	r.sequence = sequence
	if err := validateSnapshot(r.snapshotPath, sequence); err != nil {
		return err
	}
	if err := validateRecoveredState(r.plans, r.commands); err != nil {
		return fmt.Errorf("validate recovered projection: %w", err)
	}
	return nil
}

var _ rigging.Repository = (*Repository)(nil)
