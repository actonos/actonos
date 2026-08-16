package memory

import (
	"math"
	"testing"
	"time"
)

func TestDecayScore(t *testing.T) {
	tests := []struct {
		name        string
		elapsed     time.Duration
		lambdaHours float64
		accessCount int
		expectedMin float64
		expectedMax float64
	}{
		{
			name:        "instant_memory",
			elapsed:     0,
			lambdaHours: 24.0,
			accessCount: 0,
			expectedMin: 0.99,
			expectedMax: 1.00,
		},
		{
			name:        "one_half_life_past",
			elapsed:     24 * time.Hour,
			lambdaHours: 24.0,
			accessCount: 0,
			expectedMin: 0.35, // e^-1 ≈ 0.3678
			expectedMax: 0.38,
		},
		{
			name:        "spaced_repetition_boost",
			elapsed:     24 * time.Hour,
			lambdaHours: 24.0,
			accessCount: 10, // λ_effective = 24 * (1 + ln(11)) ≈ 81.55 hrs -> e^(-24/81.55) ≈ 0.745
			expectedMin: 0.70,
			expectedMax: 0.78,
		},
		{
			name:        "old_forgotten_memory",
			elapsed:     30 * 24 * time.Hour,
			lambdaHours: 24.0,
			accessCount: 0,
			expectedMin: 0.0001,
			expectedMax: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TemporalDecayFactor(tt.elapsed, tt.lambdaHours, tt.accessCount)
			if got < tt.expectedMin || got > tt.expectedMax {
				t.Fatalf("TemporalDecayFactor(%v, %v, %v) = %f, want between %f and %f",
					tt.elapsed, tt.lambdaHours, tt.accessCount, got, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestCalculateRetrievalScore(t *testing.T) {
	cfg := DefaultDecayConfig()

	// High similarity + fresh memory
	scoreFresh := CalculateRetrievalScore(0, 1.0, 0, 0.95, cfg)
	if scoreFresh < 0.85 {
		t.Fatalf("expected high retrieval score for fresh similar memory, got %f", scoreFresh)
	}

	// High similarity + very old memory
	scoreOld := CalculateRetrievalScore(30*24*time.Hour, 1.0, 0, 0.95, cfg)
	if scoreOld >= scoreFresh {
		t.Fatalf("expected old memory score (%f) to be lower than fresh memory score (%f)", scoreOld, scoreFresh)
	}

	// Verify importance weight boost
	scoreImportant := CalculateRetrievalScore(24*time.Hour, 2.0, 0, 0.8, cfg)
	scoreRegular := CalculateRetrievalScore(24*time.Hour, 1.0, 0, 0.8, cfg)
	if scoreImportant <= scoreRegular {
		t.Fatalf("expected important memory (%f) > regular memory (%f)", scoreImportant, scoreRegular)
	}
}

func TestSigmoidMath(t *testing.T) {
	if math.Abs(sigmoid(0)-0.5) > 0.001 {
		t.Fatalf("sigmoid(0) must be 0.5")
	}
	if sigmoid(10) < 0.99 {
		t.Fatalf("sigmoid(10) must be close to 1.0")
	}
	if sigmoid(-10) > 0.01 {
		t.Fatalf("sigmoid(-10) must be close to 0.0")
	}
}
