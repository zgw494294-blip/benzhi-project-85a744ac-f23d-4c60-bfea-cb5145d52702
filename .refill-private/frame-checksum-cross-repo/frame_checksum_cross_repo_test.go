package frame_checksum_cross_repo_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"stage-rig-clearance/internal/rigging"
	"stage-rig-clearance/internal/store"
)

type checksumBarrier struct {
	entered chan<- struct{}
	release <-chan struct{}
	once    sync.Once
}

func (b *checksumBarrier) MarshalJSON() ([]byte, error) {
	b.once.Do(func() {
		b.entered <- struct{}{}
		<-b.release
	})
	return json.Marshal(strings.Repeat("x", 2<<20))
}

func TestConcurrentRepositoriesDoNotShareChecksumState(t *testing.T) {
	repositories := make([]*store.Repository, 2)
	for i := range repositories {
		repository, err := store.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		repositories[i] = repository
		defer repository.Close()
	}

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	errors := make(chan error, 2)
	for i, repository := range repositories {
		index := i
		plan := &rigging.RigPlan{
			ID:              "plan-checksum-" + string(rune('a'+index)),
			VenueName:       "并发校验和测试",
			PerformanceDate: "2030-01-01",
			OwnerName:       "技师",
			Status:          rigging.StatusDraft,
			Version:         1,
			CreatedAt:       time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		barrier := &checksumBarrier{entered: entered, release: release}
		go func() {
			receipt := rigging.CommandReceipt{PlanID: plan.ID, Version: 1, Action: "plan.created"}
			errors <- repository.Commit(context.Background(), plan, 0, []rigging.DomainEvent{{
				Type: "plan.created", Actor: "技师", Payload: map[string]any{"barrier": barrier},
			}}, "checksum-key-"+plan.ID, "plan.created", receipt)
		}()
	}

	<-entered
	<-entered
	close(release)
	for range repositories {
		if err := <-errors; err != nil {
			t.Fatalf("commit failed: %v", err)
		}
	}
}
