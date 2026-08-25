package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditChainAndCredentialSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(context.Background(), "plan-1", "plan.created", "技师", map[string]any{"version": 1}); err != nil {
		t.Fatal(err)
	}
	credential, err := store.Issue(context.Background(), "plan-1", "frozen-digest", "负责人")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	records, verification, err := reopened.Timeline(context.Background(), "plan-1")
	if err != nil || !verification.Valid || len(records) != 2 {
		t.Fatalf("audit recovery failed: %+v %v", verification, err)
	}
	credentialVerification, err := reopened.VerifyCredential(context.Background(), credential)
	if err != nil || !credentialVerification.Valid {
		t.Fatalf("credential recovery failed: %+v %v", credentialVerification, err)
	}
}

func TestAuditRejectsTruncatedFrame(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(dir)
	_, _ = store.Append(context.Background(), "plan-1", "created", "actor", nil)
	_ = store.Close()
	path := filepath.Join(dir, "audit.chain")
	info, _ := os.Stat(path)
	if err := os.Truncate(path, info.Size()-1); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("truncated audit chain was accepted")
	}
}
