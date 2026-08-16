package memory

import (
	"math"
	"time"
)

const (
	// DefaultDecayHalfLifeHours is the default decay constant λ (24 hours).
	DefaultDecayHalfLifeHours = 24.0

	// DefaultAlpha is the weight given to temporal decay and importance.
	DefaultAlpha = 0.4

	// DefaultBeta is the weight given to semantic/lexical similarity.
	DefaultBeta = 0.6
)

// DecayConfig defines parameters for the Ebbinghaus Forgetting Curve.
type DecayConfig struct {
	LambdaHours float64 // Half-life stability in hours (λ)
	Alpha       float64 // Weight for temporal-importance component (α)
	Beta        float64 // Weight for similarity component (β)
}

// DefaultDecayConfig returns standard calibrated decay parameters.
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		LambdaHours: DefaultDecayHalfLifeHours,
		Alpha:       DefaultAlpha,
		Beta:        DefaultBeta,
	}
}

// TemporalDecayFactor calculates D(t) = e^(-Δt / λ_effective) based on elapsed time and access count.
// Uses spaced repetition memory consolidation: λ_effective = λ * (1 + ln(1 + accessCount)).
func TemporalDecayFactor(elapsed time.Duration, lambdaHours float64, accessCount int) float64 {
	if lambdaHours <= 0 {
		lambdaHours = DefaultDecayHalfLifeHours
	}

	// Calculate consolidated stability
	effectiveLambdaHours := lambdaHours * (1.0 + math.Log(1.0+float64(accessCount)))

	elapsedHours := elapsed.Hours()
	if elapsedHours <= 0 {
		return 1.0
	}

	decay := math.Exp(-elapsedHours / effectiveLambdaHours)
	if decay < 0.0001 {
		return 0.0001
	}
	if decay > 1.0 {
		return 1.0
	}
	return decay
}

// CalculateRetrievalScore calculates the composite score:
// R(m, q, t) = α * (D(t) * W(m)) + β * similarity
func CalculateRetrievalScore(
	elapsed time.Duration,
	importanceWeight float64,
	accessCount int,
	similarity float64,
	cfg DecayConfig,
) float64 {
	if importanceWeight <= 0 {
		importanceWeight = 1.0
	}

	// Normalize alpha & beta to sum to 1.0
	totalWeight := cfg.Alpha + cfg.Beta
	alpha := cfg.Alpha / totalWeight
	beta := cfg.Beta / totalWeight

	decay := TemporalDecayFactor(elapsed, cfg.LambdaHours, accessCount)
	temporalScore := decay * importanceWeight

	// Bound temporalScore to reasonable range [0, 2.0]
	if temporalScore > 2.0 {
		temporalScore = 2.0
	}

	// Bound similarity to [0, 1.0]
	if similarity < 0 {
		similarity = 0
	} else if similarity > 1.0 {
		similarity = 1.0
	}

	score := (alpha * temporalScore) + (beta * similarity)
	return score
}
