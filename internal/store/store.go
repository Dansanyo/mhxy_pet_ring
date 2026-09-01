package store

import (
	"context"
	"errors"
	"time"
)

var ErrDuplicate = errors.New("duplicate event")

type TaskEvent struct {
	EventID    string
	DeviceHash string
	RingNumber int
	LevelBand  int
	TaskType   string
	Score      int
	CreatedAt  time.Time
}

type RewardEvent struct {
	EventID     string
	DeviceHash  string
	LevelBand   int
	FinalScore  int
	RewardType  string
	RewardLevel int
	CreatedAt   time.Time
}

type TaskAggregate struct {
	Bucket   int    `json:"bucket"`
	TaskType string `json:"taskType"`
	Score    int    `json:"score"`
	Count    int    `json:"count"`
}

type RewardAggregate struct {
	LevelBand   int    `json:"levelBand"`
	ScoreBucket int    `json:"scoreBucket"`
	RewardType  string `json:"rewardType"`
	RewardLevel int    `json:"rewardLevel"`
	Count       int    `json:"count"`
}

type Model struct {
	TaskSamples   int               `json:"taskSamples"`
	RewardSamples int               `json:"rewardSamples"`
	TaskBuckets   []TaskAggregate   `json:"taskBuckets"`
	RewardBuckets []RewardAggregate `json:"rewardBuckets"`
	UpdatedAt     *time.Time        `json:"updatedAt"`
}

type EventStore interface {
	InsertTaskEvent(context.Context, TaskEvent) error
	InsertRewardEvent(context.Context, RewardEvent) error
	AggregateModel(context.Context) (Model, error)
}
