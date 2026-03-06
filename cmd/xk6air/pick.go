package xk6air

import (
	"fmt"
	"math/rand"

	"github.com/grafana/sobek"
	"github.com/stroppy-io/stroppy/pkg/common/generate"
)

type Picker struct {
	randomness *rand.Rand
	seed       uint64
}

func NewPicker(seed uint64) *Picker {
	seed = generate.ResolveSeed(seed)

	return &Picker{
		randomness: rand.New(rand.NewSource(int64(seed))),
		seed:       seed,
	}
}

// Pick returns a random element from the given array.
func (g *Picker) Pick(array []sobek.Value) (sobek.Value, error) {
	return array[g.randomness.Intn(len(array))], nil
}

// WeightedPick returns a random element from the given array, based on the
// given weights.
func (g *Picker) PickWeighted(array []sobek.Value, weights []float64) (sobek.Value, error) {
	if len(array) != len(weights) {
		return sobek.Undefined(), fmt.Errorf("array and weights must be of the same length")
	}

	totalWeight := 0.0
	for _, weight := range weights {
		totalWeight += weight
	}

	threshold := g.randomness.Float64() * totalWeight

	cumulativeWeight := 0.0
	for index, weight := range weights {
		cumulativeWeight += weight

		if cumulativeWeight >= threshold {
			return array[index], nil
		}
	}

	return sobek.Undefined(), fmt.Errorf("unreachable! weightedPick should never reach here")
}
