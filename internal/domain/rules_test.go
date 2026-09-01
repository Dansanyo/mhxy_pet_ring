package domain

import "testing"

func intPtr(value int) *int { return &value }

func TestTaskTypesContainKnownScores(t *testing.T) {
	want := map[string]int{
		"find_person":     1,
		"equipment_60":    2,
		"furniture_1":     2,
		"medicine":        2,
		"cooking":         2,
		"equipment_70":    3,
		"instrument":      4,
		"flower":          4,
		"equipment_80":    5,
		"furniture_2":     5,
		"mutant_specific": 10,
	}

	got := make(map[string]int)
	for _, rule := range TaskTypes() {
		got[rule.ID] = rule.BaseScore
	}

	if len(got) != len(want) {
		t.Fatalf("got %d task types, want %d", len(got), len(want))
	}
	for id, score := range want {
		if got[id] != score {
			t.Errorf("task %s score = %d, want %d", id, got[id], score)
		}
	}
}

func TestScoreResolutionSeparatesRequestedTaskFromPlayerAction(t *testing.T) {
	tests := []struct {
		resolution string
		want       int
	}{
		{ResolutionFulfilled, 10},
		{ResolutionMutantUnspecified, 5},
		{ResolutionNormalPet, -15},
		{ResolutionSkipped, -20},
	}
	for _, tt := range tests {
		got, err := ScoreResolution("mutant_specific", tt.resolution, nil, nil)
		if err != nil || got != tt.want {
			t.Errorf("ScoreResolution(%q) = %d, %v; want %d", tt.resolution, got, err, tt.want)
		}
	}
}

func TestSkipIsAvailableForEveryTask(t *testing.T) {
	got, err := ScoreResolution("equipment_80", ResolutionSkipped, nil, nil)
	if err != nil || got != -20 {
		t.Fatalf("skip score = %d, %v; want -20", got, err)
	}
}

func TestQualityResolutionOnlyRequiresInputsWhenBelowRequirement(t *testing.T) {
	got, err := ScoreResolution("medicine", ResolutionFulfilled, nil, nil)
	if err != nil || got != 2 {
		t.Fatalf("fulfilled quality task = %d, %v; want 2", got, err)
	}
	got, err = ScoreResolution("medicine", ResolutionQualityBelow, intPtr(63), intPtr(58))
	if err != nil || got != 0 {
		t.Fatalf("below-quality task = %d, %v; want 0", got, err)
	}
	if _, err := ScoreResolution("medicine", ResolutionQualityBelow, intPtr(63), intPtr(63)); err == nil {
		t.Fatal("below-quality resolution should reject quality at requirement")
	}
}

func TestMutantAlternativeRejectsUnrelatedTask(t *testing.T) {
	if _, err := ScoreResolution("flower", ResolutionNormalPet, nil, nil); err == nil {
		t.Fatal("normal pet resolution should be rejected for flower task")
	}
}

func TestScoreTaskUsesBaseScore(t *testing.T) {
	got, err := ScoreTask("mutant_specific", nil, nil)
	if err != nil {
		t.Fatalf("ScoreTask returned error: %v", err)
	}
	if got != 10 {
		t.Fatalf("ScoreTask = %d, want 10", got)
	}
}

func TestScoreTaskQualityAtRequirement(t *testing.T) {
	got, err := ScoreTask("cooking", intPtr(63), intPtr(63))
	if err != nil {
		t.Fatalf("ScoreTask returned error: %v", err)
	}
	if got != 2 {
		t.Fatalf("ScoreTask = %d, want 2", got)
	}
}

func TestScoreTaskQualityShortfallRoundsDown(t *testing.T) {
	got, err := ScoreTask("medicine", intPtr(63), intPtr(58))
	if err != nil {
		t.Fatalf("ScoreTask returned error: %v", err)
	}
	if got != 0 {
		t.Fatalf("ScoreTask = %d, want 0", got)
	}
}

func TestScoreTaskQualityCanBecomeNegative(t *testing.T) {
	got, err := ScoreTask("medicine", intPtr(70), intPtr(60))
	if err != nil {
		t.Fatalf("ScoreTask returned error: %v", err)
	}
	if got != -3 {
		t.Fatalf("ScoreTask = %d, want -3", got)
	}
}

func TestScoreTaskRequiresQualityForMedicineAndCooking(t *testing.T) {
	if _, err := ScoreTask("medicine", nil, nil); err == nil {
		t.Fatal("ScoreTask should reject missing quality")
	}
}

func TestScoreTaskRejectsUnknownTask(t *testing.T) {
	if _, err := ScoreTask("unknown", nil, nil); err == nil {
		t.Fatal("ScoreTask should reject unknown task")
	}
}

func TestRewardTierMatchesProvidedTable(t *testing.T) {
	tests := []struct {
		level int
		score int
		want  int
	}{
		{69, 197, 90},
		{89, 201, 100},
		{109, 214, 120},
		{129, 237, 150},
		{138, 174, 90},
		{155, 219, 140},
		{159, 227, 150},
		{170, 204, 130},
		{175, 202, 130},
		{175, 221, 140},
		{175, 222, 150},
		{175, 161, 0},
	}

	for _, tt := range tests {
		if got := RewardTier(tt.level, tt.score); got != tt.want {
			t.Errorf("RewardTier(%d, %d) = %d, want %d", tt.level, tt.score, got, tt.want)
		}
	}
}

func TestRewardThresholdUsesLevelFormula(t *testing.T) {
	if got := RewardThreshold(175, 150); got != 222 {
		t.Fatalf("RewardThreshold(175, 150) = %d, want 222", got)
	}
	if got := RewardThreshold(69, 90); got != 197 {
		t.Fatalf("RewardThreshold(69, 90) = %d, want 197", got)
	}
}
