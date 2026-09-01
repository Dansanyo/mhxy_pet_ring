package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreCreatesSchemaAndAggregatesEvents(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "pet-ring.db")
	db, err := Open(databasePath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	deviceA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	deviceB := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	events := []TaskEvent{
		{EventID: "task-1", DeviceHash: deviceA, RingNumber: 1, LevelBand: 175, TaskType: "find_person", Score: 1, CreatedAt: now},
		{EventID: "task-2", DeviceHash: deviceB, RingNumber: 8, LevelBand: 175, TaskType: "find_person", Score: 1, CreatedAt: now.Add(time.Minute)},
		{EventID: "task-3", DeviceHash: deviceA, RingNumber: 12, LevelBand: 175, TaskType: "equipment_80", Score: 5, CreatedAt: now.Add(2 * time.Minute)},
	}
	for _, event := range events {
		if err := db.InsertTaskEvent(ctx, event); err != nil {
			t.Fatalf("InsertTaskEvent: %v", err)
		}
	}

	if err := db.InsertRewardEvent(ctx, RewardEvent{
		EventID: "reward-1", DeviceHash: deviceA, LevelBand: 175,
		FinalScore: 202, RewardType: "book", RewardLevel: 130, CreatedAt: now,
	}); err != nil {
		t.Fatalf("InsertRewardEvent: %v", err)
	}

	model, err := db.AggregateModel(ctx)
	if err != nil {
		t.Fatalf("AggregateModel: %v", err)
	}
	if model.TaskSamples != 3 || model.RewardSamples != 1 {
		t.Fatalf("sample counts = %d/%d, want 3/1", model.TaskSamples, model.RewardSamples)
	}
	if len(model.TaskBuckets) != 2 {
		t.Fatalf("task aggregate rows = %d, want 2", len(model.TaskBuckets))
	}
	if got := model.TaskBuckets[0]; got.Bucket != 1 || got.TaskType != "find_person" || got.Count != 2 {
		t.Fatalf("first task aggregate = %+v", got)
	}
	if len(model.RewardBuckets) != 1 || model.RewardBuckets[0].ScoreBucket != 200 {
		t.Fatalf("reward aggregates = %+v", model.RewardBuckets)
	}
}

func TestSQLiteStoreRejectsDuplicateEventID(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pet-ring.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	event := TaskEvent{
		EventID: "same-event", DeviceHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RingNumber: 1,
		LevelBand: 175, TaskType: "find_person", Score: 1, CreatedAt: time.Now(),
	}
	if err := db.InsertTaskEvent(context.Background(), event); err != nil {
		t.Fatalf("first InsertTaskEvent: %v", err)
	}
	if err := db.InsertTaskEvent(context.Background(), event); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v, want ErrDuplicate", err)
	}
}

func TestSQLiteStoreRejectsInvalidDatabaseEvent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pet-ring.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	err = db.InsertTaskEvent(context.Background(), TaskEvent{
		EventID: "bad-ring", DeviceHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RingNumber: 101,
		LevelBand: 175, TaskType: "find_person", Score: 1, CreatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("invalid ring should fail")
	}
}
