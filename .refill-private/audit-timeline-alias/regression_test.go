package audit_timeline_alias_test

import (
	"context"
	"testing"

	"stage-rig-clearance/internal/audit"
)

func TestTimelineResultCannotCorruptStoredAuditChain(t *testing.T) {
	store, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Append(context.Background(), "plan-a", "plan.created", "甲", map[string]any{"version": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), "plan-b", "plan.created", "乙", map[string]any{"version": 1}); err != nil {
		t.Fatal(err)
	}
	records, verification, err := store.Timeline(context.Background(), "plan-a")
	if err != nil || !verification.Valid || len(records) != 1 {
		t.Fatalf("initial timeline failed: records=%d verification=%+v err=%v", len(records), verification, err)
	}
	records[0].Payload["version"] = 999
	_, after, err := store.Timeline(context.Background(), "plan-b")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Valid {
		t.Fatalf("mutating a returned timeline corrupted the stored global audit chain: %+v", after)
	}
}
