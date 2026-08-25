package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"stage-rig-clearance/internal/rigging"
)

type commandRecord struct {
	Action  string                 `json:"action"`
	Receipt rigging.CommandReceipt `json:"receipt"`
}

type projectionSnapshot struct {
	SchemaVersion int                         `json:"schemaVersion"`
	LastSequence  uint64                      `json:"lastSequence"`
	Plans         map[string]*rigging.RigPlan `json:"plans"`
	Commands      map[string]commandRecord    `json:"commands"`
	Checksum      string                      `json:"checksum"`
}

func snapshotChecksum(snapshot projectionSnapshot) (string, error) {
	snapshot.Checksum = ""
	b, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (r *Repository) writeSnapshotLocked() error {
	snapshot := projectionSnapshot{SchemaVersion: schemaVersion, LastSequence: r.sequence, Plans: r.plans, Commands: r.commands}
	checksum, err := snapshotChecksum(snapshot)
	if err != nil {
		return err
	}
	snapshot.Checksum = checksum
	b, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(r.dir, ".projection-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	clean := func() { _ = tmp.Close(); _ = os.Remove(name) }
	if _, err := tmp.Write(b); err != nil {
		clean()
		return err
	}
	if err := tmp.Sync(); err != nil {
		clean()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, r.snapshotPath); err != nil {
		_ = os.Remove(name)
		return err
	}
	dir, err := os.Open(filepath.Dir(r.snapshotPath))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func validateSnapshot(path string, expectedSequence uint64) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var snapshot projectionSnapshot
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return fmt.Errorf("decode projection snapshot: %w", err)
	}
	if snapshot.SchemaVersion != schemaVersion {
		return fmt.Errorf("unsupported snapshot schemaVersion %d", snapshot.SchemaVersion)
	}
	checksum, err := snapshotChecksum(snapshot)
	if err != nil {
		return err
	}
	if checksum != snapshot.Checksum {
		return fmt.Errorf("projection snapshot checksum mismatch")
	}
	if snapshot.LastSequence > expectedSequence {
		return fmt.Errorf("projection snapshot is ahead of event log")
	}
	return nil
}
