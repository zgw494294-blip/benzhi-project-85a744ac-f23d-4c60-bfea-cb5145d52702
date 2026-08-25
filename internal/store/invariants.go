package store

import (
	"fmt"

	"stage-rig-clearance/internal/rigging"
)

func validatePlanProjection(plan *rigging.RigPlan) error {
	if plan == nil {
		return fmt.Errorf("nil aggregate projection")
	}
	if plan.ID == "" {
		return fmt.Errorf("aggregate has empty id")
	}
	if plan.Version < 1 {
		return fmt.Errorf("aggregate %s has invalid version %d", plan.ID, plan.Version)
	}
	switch plan.Status {
	case rigging.StatusDraft, rigging.StatusTesting, rigging.StatusRemediation, rigging.StatusApproved, rigging.StatusReturned:
	default:
		return fmt.Errorf("aggregate %s has unknown status %q", plan.ID, plan.Status)
	}
	if plan.Status == rigging.StatusApproved {
		if plan.FrozenAt == nil || plan.FrozenDigest == "" {
			return fmt.Errorf("approved aggregate %s is not frozen", plan.ID)
		}
		digest, err := rigging.FrozenDigest(plan)
		if err != nil {
			return fmt.Errorf("calculate aggregate %s frozen digest: %w", plan.ID, err)
		}
		if digest != plan.FrozenDigest {
			return fmt.Errorf("aggregate %s frozen digest mismatch", plan.ID)
		}
		if len(plan.FrozenPoints) != len(plan.Points) || len(plan.FrozenTests) != len(plan.Tests) {
			return fmt.Errorf("aggregate %s frozen manifest count mismatch", plan.ID)
		}
	} else if plan.FrozenAt != nil || plan.FrozenDigest != "" {
		return fmt.Errorf("mutable aggregate %s unexpectedly contains a freeze marker", plan.ID)
	}
	pointIDs := make(map[string]struct{}, len(plan.Points))
	labels := make(map[string]struct{}, len(plan.Points))
	for _, point := range plan.Points {
		if point.ID == "" || point.PlanID != plan.ID {
			return fmt.Errorf("aggregate %s has an invalid point identity", plan.ID)
		}
		if _, exists := pointIDs[point.ID]; exists {
			return fmt.Errorf("aggregate %s repeats point %s", plan.ID, point.ID)
		}
		if _, exists := labels[point.Label]; exists {
			return fmt.Errorf("aggregate %s repeats point label %s", plan.ID, point.Label)
		}
		pointIDs[point.ID] = struct{}{}
		labels[point.Label] = struct{}{}
	}
	for _, point := range plan.Points {
		if point.RedundantPointID != "" {
			if point.RedundantPointID == point.ID {
				return fmt.Errorf("point %s is its own redundancy", point.ID)
			}
			if _, exists := pointIDs[point.RedundantPointID]; !exists {
				return fmt.Errorf("point %s references missing redundancy %s", point.ID, point.RedundantPointID)
			}
		}
		if point.PrimaryPointID != "" {
			if _, exists := pointIDs[point.PrimaryPointID]; !exists {
				return fmt.Errorf("point %s references missing primary point %s", point.ID, point.PrimaryPointID)
			}
		}
	}
	testIDs := make(map[string]struct{}, len(plan.Tests))
	for _, test := range plan.Tests {
		if test.ID == "" || test.PlanID != plan.ID {
			return fmt.Errorf("aggregate %s has an invalid test identity", plan.ID)
		}
		if _, exists := testIDs[test.ID]; exists {
			return fmt.Errorf("aggregate %s repeats test %s", plan.ID, test.ID)
		}
		if _, exists := pointIDs[test.PointID]; !exists {
			return fmt.Errorf("test %s references missing point %s", test.ID, test.PointID)
		}
		testIDs[test.ID] = struct{}{}
	}
	taskIDs := make(map[string]struct{}, len(plan.RetestTasks))
	for _, task := range plan.RetestTasks {
		if task.ID == "" || task.PlanID != plan.ID {
			return fmt.Errorf("aggregate %s has an invalid retest task identity", plan.ID)
		}
		if _, exists := taskIDs[task.ID]; exists {
			return fmt.Errorf("aggregate %s repeats retest task %s", plan.ID, task.ID)
		}
		if _, exists := pointIDs[task.PointID]; !exists {
			return fmt.Errorf("retest task %s references missing point %s", task.ID, task.PointID)
		}
		taskIDs[task.ID] = struct{}{}
	}
	issueIDs := make(map[string]struct{}, len(plan.Issues))
	for _, issue := range plan.Issues {
		if issue.ID == "" || issue.PlanID != plan.ID {
			return fmt.Errorf("aggregate %s has an invalid issue identity", plan.ID)
		}
		if _, exists := issueIDs[issue.ID]; exists {
			return fmt.Errorf("aggregate %s repeats issue %s", plan.ID, issue.ID)
		}
		if issue.PointID != "" {
			if _, exists := pointIDs[issue.PointID]; !exists {
				return fmt.Errorf("issue %s references missing point %s", issue.ID, issue.PointID)
			}
		}
		issueIDs[issue.ID] = struct{}{}
	}
	credentialIDs := make(map[string]struct{}, len(plan.Credentials))
	for _, credential := range plan.Credentials {
		if credential.ID == "" || credential.PlanID != plan.ID {
			return fmt.Errorf("aggregate %s has an invalid credential identity", plan.ID)
		}
		if _, exists := credentialIDs[credential.ID]; exists {
			return fmt.Errorf("aggregate %s repeats credential %s", plan.ID, credential.ID)
		}
		if credential.FrozenDigest != plan.FrozenDigest {
			return fmt.Errorf("credential %s references another frozen digest", credential.ID)
		}
		credentialIDs[credential.ID] = struct{}{}
	}
	return nil
}

func validateRecoveredState(plans map[string]*rigging.RigPlan, commands map[string]commandRecord) error {
	for id, plan := range plans {
		if id != plan.ID {
			return fmt.Errorf("projection map key %s differs from aggregate id %s", id, plan.ID)
		}
		if err := validatePlanProjection(plan); err != nil {
			return err
		}
	}
	for key, command := range commands {
		if key == "" || command.Action == "" || command.Receipt.Action != command.Action {
			return fmt.Errorf("invalid recovered command receipt for key %q", key)
		}
		plan, exists := plans[command.Receipt.PlanID]
		if !exists {
			return fmt.Errorf("command key %s references missing aggregate %s", key, command.Receipt.PlanID)
		}
		if command.Receipt.Version < 1 || command.Receipt.Version > plan.Version {
			return fmt.Errorf("command key %s has invalid result version %d", key, command.Receipt.Version)
		}
	}
	return nil
}
