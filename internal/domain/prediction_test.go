package domain

import (
	"math"
	"testing"
)

func TestFallbackProjection(t *testing.T) {
	got := FallbackProjection(75, 137, 712.8, 175)
	if got.ExpectedScore != 183 {
		t.Fatalf("ExpectedScore = %d, want 183", got.ExpectedScore)
	}
	if math.Abs(got.ExpectedCost-950.4) > 0.001 {
		t.Fatalf("ExpectedCost = %.3f, want 950.4", got.ExpectedCost)
	}
	if got.Confidence != "low" {
		t.Fatalf("Confidence = %q, want low", got.Confidence)
	}
	if got.ExpectedTier != 110 {
		t.Fatalf("ExpectedTier = %d, want 110", got.ExpectedTier)
	}
}

func TestFallbackProjectionHandlesEmptyCycle(t *testing.T) {
	got := FallbackProjection(0, 0, 0, 175)
	if got.ExpectedScore != 0 || got.ExpectedCost != 0 || got.ExpectedTier != 0 {
		t.Fatalf("empty projection = %+v", got)
	}
}

func TestSimulateProjectionWithDeterministicOutcomes(t *testing.T) {
	got := SimulateProjection(SimulationInput{
		CurrentRing:  98,
		CurrentScore: 180,
		CurrentCost:  900,
		PlayerLevel:  175,
		Runs:         100,
		Buckets: map[int][]WeightedTaskOutcome{
			10: {{TaskType: "find_person", Score: 1, Count: 50}},
		},
		LocalPrices: map[string]float64{"find_person": 0},
	}, 42)

	if got.ExpectedScore != 182 || got.P10Score != 182 || got.P50Score != 182 || got.P90Score != 182 {
		t.Fatalf("score projection = %+v", got)
	}
	if got.ExpectedTier != 110 {
		t.Fatalf("ExpectedTier = %d, want 110", got.ExpectedTier)
	}
	for _, tier := range []int{90, 100, 110} {
		if got.TierProbabilities[tier] != 1 {
			t.Errorf("tier %d probability = %v, want 1", tier, got.TierProbabilities[tier])
		}
	}
	if got.TierProbabilities[120] != 0 {
		t.Errorf("tier 120 probability = %v, want 0", got.TierProbabilities[120])
	}
}

func TestSimulateProjectionUsesGlobalFallbackBucket(t *testing.T) {
	got := SimulateProjection(SimulationInput{
		CurrentRing:  99,
		CurrentScore: 160,
		PlayerLevel:  175,
		Runs:         10,
		Buckets: map[int][]WeightedTaskOutcome{
			0: {{TaskType: "mutant_specific", Score: 10, Count: 1}},
		},
	}, 7)
	if got.ExpectedScore != 170 {
		t.Fatalf("ExpectedScore = %d, want 170", got.ExpectedScore)
	}
}

func TestSimulateProjectionFallsBackWithoutSamples(t *testing.T) {
	got := SimulateProjection(SimulationInput{
		CurrentRing:  75,
		CurrentScore: 137,
		CurrentCost:  712.8,
		PlayerLevel:  175,
	}, 1)
	if got.ExpectedScore != 183 || got.Confidence != "low" {
		t.Fatalf("projection = %+v", got)
	}
}
