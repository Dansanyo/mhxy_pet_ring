package store

import (
	"context"
	"database/sql"
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

func TestDeleteTaskEventsOnlyDeletesMatchingDevice(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pet-ring.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, event := range []TaskEvent{
		{EventID: "delete-one", DeviceHash: "aaaaaaaaaaaaaaaa", RingNumber: 1, LevelBand: 175, TaskType: "find_person", Score: 1, CreatedAt: time.Now()},
		{EventID: "keep-other", DeviceHash: "bbbbbbbbbbbbbbbb", RingNumber: 2, LevelBand: 175, TaskType: "flower_instrument", Score: 4, CreatedAt: time.Now()},
	} {
		if err := db.InsertTaskEvent(ctx, event); err != nil {
			t.Fatalf("InsertTaskEvent: %v", err)
		}
	}
	deleted, err := db.DeleteTaskEvents(ctx, "aaaaaaaaaaaaaaaa", []string{"delete-one", "keep-other"})
	if err != nil {
		t.Fatalf("DeleteTaskEvents: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	model, err := db.AggregateModel(ctx)
	if err != nil {
		t.Fatalf("AggregateModel: %v", err)
	}
	if model.TaskSamples != 1 || model.TaskBuckets[0].TaskType != "flower_instrument" {
		t.Fatalf("model after delete = %+v", model)
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

func TestSQLiteStoreMigratesLegacyMutantResolution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	_, err = legacy.Exec(`
		CREATE TABLE task_events (
			event_id TEXT PRIMARY KEY,
			device_hash TEXT NOT NULL,
			ring_number INTEGER NOT NULL,
			level_band INTEGER NOT NULL,
			task_type TEXT NOT NULL,
			score INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
		INSERT INTO task_events VALUES (
			'legacy-mutant', 'aaaaaaaaaaaaaaaa', 20, 175,
			'normal_pet_as_mutant', -15, '2026-09-01T00:00:00Z'
		);
		INSERT INTO task_events VALUES (
			'legacy-flower', 'aaaaaaaaaaaaaaaa', 21, 175,
			'flower', 4, '2026-09-01T00:01:00Z'
		);
		INSERT INTO task_events VALUES (
			'legacy-medicine', 'aaaaaaaaaaaaaaaa', 22, 175,
			'medicine', -3, '2026-09-01T00:02:00Z'
		);`)
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	defer db.Close()

	var taskType, resolution string
	var requestedScore, score int
	err = db.db.QueryRow(`SELECT task_type, resolution, requested_score, score FROM task_events WHERE event_id = 'legacy-mutant'`).Scan(
		&taskType, &resolution, &requestedScore, &score,
	)
	if err != nil {
		t.Fatalf("read migrated event: %v", err)
	}
	if taskType != "mutant_specific" || resolution != "normal_pet" || requestedScore != 10 || score != -15 {
		t.Fatalf("migrated event = %q/%q/%d/%d", taskType, resolution, requestedScore, score)
	}
	if err := db.db.QueryRow(`SELECT task_type FROM task_events WHERE event_id = 'legacy-flower'`).Scan(&taskType); err != nil {
		t.Fatalf("read migrated flower event: %v", err)
	}
	if taskType != "flower_instrument" {
		t.Fatalf("migrated flower task = %q", taskType)
	}
	if err := db.db.QueryRow(`SELECT requested_score, score FROM task_events WHERE event_id = 'legacy-medicine'`).Scan(&requestedScore, &score); err != nil {
		t.Fatalf("read migrated medicine event: %v", err)
	}
	if requestedScore != -3 || score != -3 {
		t.Fatalf("migrated medicine scores = %d/%d, want -3/-3", requestedScore, score)
	}
}
