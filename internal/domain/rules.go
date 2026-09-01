package domain

import (
	"errors"
	"fmt"
	"math"
)

type TaskRule struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BaseScore    int    `json:"baseScore"`
	NeedsQuality bool   `json:"needsQuality"`
}

var taskRules = []TaskRule{
	{ID: "find_person", Name: "找人", BaseScore: 1},
	{ID: "equipment_60", Name: "60级装备", BaseScore: 2},
	{ID: "furniture_1", Name: "1级家具", BaseScore: 2},
	{ID: "medicine", Name: "三级药", BaseScore: 2, NeedsQuality: true},
	{ID: "cooking", Name: "烹饪", BaseScore: 2, NeedsQuality: true},
	{ID: "equipment_70", Name: "70级装备", BaseScore: 3},
	{ID: "instrument", Name: "乐器", BaseScore: 4},
	{ID: "flower", Name: "花", BaseScore: 4},
	{ID: "equipment_80", Name: "80级装备", BaseScore: 5},
	{ID: "furniture_2", Name: "2级家具", BaseScore: 5},
	{ID: "mutant_specific", Name: "指定变异召唤兽", BaseScore: 10},
}

const (
	ResolutionFulfilled         = "fulfilled"
	ResolutionMutantUnspecified = "mutant_unspecified"
	ResolutionNormalPet         = "normal_pet"
	ResolutionSkipped           = "skipped"
)

var taskRuleByID = func() map[string]TaskRule {
	rules := make(map[string]TaskRule, len(taskRules))
	for _, rule := range taskRules {
		rules[rule.ID] = rule
	}
	return rules
}()

func TaskTypes() []TaskRule {
	result := make([]TaskRule, len(taskRules))
	copy(result, taskRules)
	return result
}

func ScoreTask(taskType string, requiredQuality, actualQuality *int) (int, error) {
	rule, ok := taskRuleByID[taskType]
	if !ok {
		return 0, fmt.Errorf("unknown task type %q", taskType)
	}
	if !rule.NeedsQuality {
		return rule.BaseScore, nil
	}
	if requiredQuality == nil || actualQuality == nil {
		return 0, errors.New("required and actual quality are required")
	}
	if *requiredQuality < 0 || *actualQuality < 0 {
		return 0, errors.New("quality cannot be negative")
	}
	if *actualQuality >= *requiredQuality {
		return rule.BaseScore, nil
	}
	return rule.BaseScore - (*requiredQuality-*actualQuality)/2, nil
}

func ScoreResolution(taskType, resolution string, requiredQuality, actualQuality *int) (int, error) {
	if resolution == "" {
		resolution = ResolutionFulfilled
	}
	if _, ok := taskRuleByID[taskType]; !ok {
		return 0, fmt.Errorf("unknown task type %q", taskType)
	}
	switch resolution {
	case ResolutionFulfilled:
		return ScoreTask(taskType, requiredQuality, actualQuality)
	case ResolutionSkipped:
		return -20, nil
	case ResolutionMutantUnspecified:
		if taskType == "mutant_specific" {
			return 5, nil
		}
	case ResolutionNormalPet:
		if taskType == "mutant_specific" {
			return -15, nil
		}
	}
	return 0, fmt.Errorf("resolution %q is not valid for task %q", resolution, taskType)
}

func RequestedTaskScore(taskType string, requiredQuality, actualQuality *int) (int, error) {
	rule, ok := taskRuleByID[taskType]
	if !ok {
		return 0, fmt.Errorf("unknown task type %q", taskType)
	}
	if !rule.NeedsQuality || requiredQuality == nil || actualQuality == nil {
		return rule.BaseScore, nil
	}
	return ScoreTask(taskType, requiredQuality, actualQuality)
}

func RewardThreshold(playerLevel, rewardLevel int) int {
	if playerLevel <= 0 || rewardLevel < 90 || rewardLevel > 150 || rewardLevel%10 != 0 {
		return 0
	}
	base := int(math.Ceil(220 - float64(playerLevel)/3))
	return base + rewardLevel - 90
}

func RewardTier(playerLevel, finalScore int) int {
	for rewardLevel := 150; rewardLevel >= 90; rewardLevel -= 10 {
		if finalScore >= RewardThreshold(playerLevel, rewardLevel) {
			return rewardLevel
		}
	}
	return 0
}
