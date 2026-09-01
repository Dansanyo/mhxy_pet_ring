package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func Open(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteStore{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) initialize(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS task_events (
			event_id TEXT PRIMARY KEY,
			device_hash TEXT NOT NULL CHECK(length(device_hash) BETWEEN 16 AND 128),
			ring_number INTEGER NOT NULL CHECK(ring_number BETWEEN 1 AND 100),
			level_band INTEGER NOT NULL CHECK(level_band BETWEEN 1 AND 200),
			task_type TEXT NOT NULL CHECK(length(task_type) BETWEEN 1 AND 64),
			resolution TEXT NOT NULL DEFAULT 'fulfilled' CHECK(length(resolution) BETWEEN 1 AND 64),
			requested_score INTEGER NOT NULL DEFAULT 0 CHECK(requested_score BETWEEN -100 AND 100),
			score INTEGER NOT NULL CHECK(score BETWEEN -100 AND 100),
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_events_bucket ON task_events(ring_number, task_type, score)`,
		`CREATE TABLE IF NOT EXISTS reward_events (
			event_id TEXT PRIMARY KEY,
			device_hash TEXT NOT NULL CHECK(length(device_hash) BETWEEN 16 AND 128),
			level_band INTEGER NOT NULL CHECK(level_band BETWEEN 1 AND 200),
			final_score INTEGER NOT NULL CHECK(final_score BETWEEN -1000 AND 2000),
			reward_type TEXT NOT NULL CHECK(length(reward_type) BETWEEN 1 AND 64),
			reward_level INTEGER NOT NULL CHECK(reward_level BETWEEN 0 AND 200),
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reward_events_model ON reward_events(level_band, final_score, reward_type, reward_level)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite: %w", err)
		}
	}
	if err := s.ensureTaskEventColumn(ctx, "resolution", "TEXT NOT NULL DEFAULT 'fulfilled'"); err != nil {
		return err
	}
	if err := s.ensureTaskEventColumn(ctx, "requested_score", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE task_events
		SET resolution = CASE task_type
			WHEN 'mutant_unspecified' THEN 'mutant_unspecified'
			WHEN 'normal_pet_as_mutant' THEN 'normal_pet'
			ELSE resolution
		END,
		task_type = CASE
			WHEN task_type IN ('mutant_unspecified', 'normal_pet_as_mutant') THEN 'mutant_specific'
			ELSE task_type
		END,
		requested_score = CASE
			WHEN task_type IN ('mutant_unspecified', 'normal_pet_as_mutant', 'mutant_specific') THEN 10
			WHEN requested_score = 0 THEN score
			ELSE requested_score
		END`); err != nil {
		return fmt.Errorf("migrate task events: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE task_events
		SET task_type = 'flower_instrument'
		WHERE task_type IN ('flower', 'instrument')`); err != nil {
		return fmt.Errorf("merge flower and instrument events: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureTaskEventColumn(ctx context.Context, name, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(task_events)`)
	if err != nil {
		return fmt.Errorf("inspect task event schema: %w", err)
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var columnName, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan task event schema: %w", err)
		}
		found = found || columnName == name
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close task event schema: %w", err)
	}
	if found {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE task_events ADD COLUMN `+name+` `+definition); err != nil {
		return fmt.Errorf("add task event column %s: %w", name, err)
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) InsertTaskEvent(ctx context.Context, event TaskEvent) error {
	if event.Resolution == "" {
		event.Resolution = "fulfilled"
		if event.RequestedScore == 0 {
			event.RequestedScore = event.Score
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_events(event_id, device_hash, ring_number, level_band, task_type, resolution, requested_score, score, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.DeviceHash, event.RingNumber, event.LevelBand, event.TaskType, event.Resolution,
		event.RequestedScore, event.Score, event.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return normalizeInsertError(err)
}

func (s *SQLiteStore) DeleteTaskEvents(ctx context.Context, deviceHash string, eventIDs []string) (int64, error) {
	if len(eventIDs) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(eventIDs)), ",")
	arguments := make([]any, 0, len(eventIDs)+1)
	arguments = append(arguments, deviceHash)
	for _, eventID := range eventIDs {
		arguments = append(arguments, eventID)
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM task_events WHERE device_hash = ? AND event_id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return 0, fmt.Errorf("delete task events: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted task events: %w", err)
	}
	return deleted, nil
}

func (s *SQLiteStore) InsertRewardEvent(ctx context.Context, event RewardEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reward_events(event_id, device_hash, level_band, final_score, reward_type, reward_level, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.DeviceHash, event.LevelBand, event.FinalScore, event.RewardType, event.RewardLevel, event.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return normalizeInsertError(err)
}

func (s *SQLiteStore) AggregateModel(ctx context.Context) (Model, error) {
	model := Model{TaskBuckets: []TaskAggregate{}, RewardBuckets: []RewardAggregate{}}
	rows, err := s.db.QueryContext(ctx, `
		SELECT CAST((ring_number - 1) / 10 AS INTEGER) + 1 AS bucket, task_type, requested_score, COUNT(*)
		FROM task_events
		GROUP BY bucket, task_type, requested_score
		ORDER BY bucket, task_type, requested_score`)
	if err != nil {
		return model, fmt.Errorf("aggregate task events: %w", err)
	}
	for rows.Next() {
		var aggregate TaskAggregate
		if err := rows.Scan(&aggregate.Bucket, &aggregate.TaskType, &aggregate.Score, &aggregate.Count); err != nil {
			_ = rows.Close()
			return model, fmt.Errorf("scan task aggregate: %w", err)
		}
		model.TaskBuckets = append(model.TaskBuckets, aggregate)
		model.TaskSamples += aggregate.Count
	}
	if err := rows.Close(); err != nil {
		return model, fmt.Errorf("close task aggregates: %w", err)
	}

	rewardRows, err := s.db.QueryContext(ctx, `
		SELECT level_band, CAST(final_score / 10 AS INTEGER) * 10 AS score_bucket, reward_type, reward_level, COUNT(*)
		FROM reward_events
		GROUP BY level_band, score_bucket, reward_type, reward_level
		ORDER BY level_band, score_bucket, reward_type, reward_level`)
	if err != nil {
		return model, fmt.Errorf("aggregate reward events: %w", err)
	}
	for rewardRows.Next() {
		var aggregate RewardAggregate
		if err := rewardRows.Scan(&aggregate.LevelBand, &aggregate.ScoreBucket, &aggregate.RewardType, &aggregate.RewardLevel, &aggregate.Count); err != nil {
			_ = rewardRows.Close()
			return model, fmt.Errorf("scan reward aggregate: %w", err)
		}
		model.RewardBuckets = append(model.RewardBuckets, aggregate)
		model.RewardSamples += aggregate.Count
	}
	if err := rewardRows.Close(); err != nil {
		return model, fmt.Errorf("close reward aggregates: %w", err)
	}

	var latest sql.NullString
	if err := s.db.QueryRowContext(ctx, `
		SELECT MAX(created_at) FROM (
			SELECT created_at FROM task_events
			UNION ALL
			SELECT created_at FROM reward_events
		)`).Scan(&latest); err != nil {
		return model, fmt.Errorf("query model timestamp: %w", err)
	}
	if latest.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, latest.String)
		if err == nil {
			model.UpdatedAt = &parsed
		}
	}
	return model, nil
}

func normalizeInsertError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
		return fmt.Errorf("%w: %v", ErrDuplicate, err)
	}
	return err
}
