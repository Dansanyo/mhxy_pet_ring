package domain

import (
	"math"
	"math/rand"
	"sort"
)

type WeightedTaskOutcome struct {
	TaskType string `json:"taskType"`
	Score    int    `json:"score"`
	Count    int    `json:"count"`
}

type SimulationInput struct {
	CurrentRing  int
	CurrentScore int
	CurrentCost  float64
	PlayerLevel  int
	Runs         int
	Buckets      map[int][]WeightedTaskOutcome
	LocalPrices  map[string]float64
}

type Projection struct {
	ExpectedScore     int             `json:"expectedScore"`
	P10Score          int             `json:"p10Score"`
	P50Score          int             `json:"p50Score"`
	P90Score          int             `json:"p90Score"`
	ExpectedCost      float64         `json:"expectedCost"`
	ExpectedTier      int             `json:"expectedTier"`
	TierProbabilities map[int]float64 `json:"tierProbabilities"`
	Confidence        string          `json:"confidence"`
	SampleCount       int             `json:"sampleCount"`
}

func FallbackProjection(currentRing, currentScore int, currentCost float64, playerLevel int) Projection {
	result := Projection{
		TierProbabilities: emptyTierProbabilities(),
		Confidence:        "low",
	}
	if currentRing <= 0 {
		return result
	}
	if currentRing > 100 {
		currentRing = 100
	}
	result.ExpectedScore = int(math.Round(float64(currentScore) / float64(currentRing) * 100))
	result.P10Score = result.ExpectedScore
	result.P50Score = result.ExpectedScore
	result.P90Score = result.ExpectedScore
	result.ExpectedCost = currentCost / float64(currentRing) * 100
	result.ExpectedTier = RewardTier(playerLevel, result.ExpectedScore)
	return result
}

func SimulateProjection(input SimulationInput, seed int64) Projection {
	sampleCount := totalSamples(input.Buckets)
	if sampleCount == 0 {
		return FallbackProjection(input.CurrentRing, input.CurrentScore, input.CurrentCost, input.PlayerLevel)
	}
	runs := input.Runs
	if runs <= 0 {
		runs = 5000
	}
	currentRing := input.CurrentRing
	if currentRing < 0 {
		currentRing = 0
	}
	if currentRing > 100 {
		currentRing = 100
	}

	rng := rand.New(rand.NewSource(seed))
	scores := make([]int, runs)
	costs := make([]float64, runs)
	tierHits := make(map[int]int, 7)
	for run := 0; run < runs; run++ {
		score := input.CurrentScore
		cost := input.CurrentCost
		valid := true
		for ring := currentRing + 1; ring <= 100; ring++ {
			bucket := (ring-1)/10 + 1
			outcomes := input.Buckets[bucket]
			if weightedTotal(outcomes) == 0 {
				outcomes = input.Buckets[0]
			}
			outcome, ok := chooseOutcome(rng, outcomes)
			if !ok {
				valid = false
				break
			}
			score += outcome.Score
			cost += input.LocalPrices[outcome.TaskType]
		}
		if !valid {
			return FallbackProjection(input.CurrentRing, input.CurrentScore, input.CurrentCost, input.PlayerLevel)
		}
		scores[run] = score
		costs[run] = cost
		for tier := 90; tier <= 150; tier += 10 {
			if score >= RewardThreshold(input.PlayerLevel, tier) {
				tierHits[tier]++
			}
		}
	}

	sort.Ints(scores)
	totalScore := 0
	totalCost := 0.0
	for index := range scores {
		totalScore += scores[index]
		totalCost += costs[index]
	}
	expectedScore := int(math.Round(float64(totalScore) / float64(runs)))
	result := Projection{
		ExpectedScore:     expectedScore,
		P10Score:          percentile(scores, 0.10),
		P50Score:          percentile(scores, 0.50),
		P90Score:          percentile(scores, 0.90),
		ExpectedCost:      totalCost / float64(runs),
		ExpectedTier:      RewardTier(input.PlayerLevel, expectedScore),
		TierProbabilities: emptyTierProbabilities(),
		Confidence:        confidenceForSamples(sampleCount),
		SampleCount:       sampleCount,
	}
	for tier := 90; tier <= 150; tier += 10 {
		result.TierProbabilities[tier] = float64(tierHits[tier]) / float64(runs)
	}
	return result
}

func chooseOutcome(rng *rand.Rand, outcomes []WeightedTaskOutcome) (WeightedTaskOutcome, bool) {
	total := weightedTotal(outcomes)
	if total <= 0 {
		return WeightedTaskOutcome{}, false
	}
	pick := rng.Intn(total)
	for _, outcome := range outcomes {
		if outcome.Count <= 0 {
			continue
		}
		if pick < outcome.Count {
			return outcome, true
		}
		pick -= outcome.Count
	}
	return WeightedTaskOutcome{}, false
}

func weightedTotal(outcomes []WeightedTaskOutcome) int {
	total := 0
	for _, outcome := range outcomes {
		if outcome.Count > 0 {
			total += outcome.Count
		}
	}
	return total
}

func totalSamples(buckets map[int][]WeightedTaskOutcome) int {
	total := 0
	for _, outcomes := range buckets {
		total += weightedTotal(outcomes)
	}
	return total
}

func percentile(sorted []int, value float64) int {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Round(value * float64(len(sorted)-1)))
	return sorted[index]
}

func confidenceForSamples(samples int) string {
	switch {
	case samples >= 1000:
		return "high"
	case samples >= 200:
		return "medium"
	default:
		return "low"
	}
}

func emptyTierProbabilities() map[int]float64 {
	result := make(map[int]float64, 7)
	for tier := 90; tier <= 150; tier += 10 {
		result[tier] = 0
	}
	return result
}
