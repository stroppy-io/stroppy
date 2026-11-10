package tpch

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const querySeedModulus int64 = 2147483647

type queryRNG int64

func (r *queryRNG) next() int64 {
	*r = queryRNG(int64(*r) * 16807 % querySeedModulus)

	return int64(*r)
}

func (r *queryRNG) between(low, high int) int {
	return low + int(float64(r.next())/float64(querySeedModulus)*float64(high-low+1))
}

func (r *queryRNG) permutation(count int) []int {
	values := make([]int, count)
	for i := range values {
		values[i] = i
	}

	for i := range values {
		j := r.between(i, count-1)
		values[i], values[j] = values[j], values[i]
	}

	return values
}

type queryDistribution struct {
	Rows []struct {
		Values  []string `json:"values"`
		Weights []int    `json:"weights"`
	} `json:"rows"`
}

var queryDistributions = sync.OnceValues(func() (map[string]queryDistribution, error) {
	data, err := files.ReadFile("distributions.json")
	if err != nil {
		return nil, err
	}

	var document struct {
		Distributions map[string]queryDistribution `json:"distributions"`
	}

	err = json.Unmarshal(data, &document)

	return document.Distributions, err
})

func (r *queryRNG) pick(d queryDistribution) (index int, value string) {
	total := 0
	for _, row := range d.Rows {
		total += row.Weights[0]
	}

	draw := r.between(1, total)
	for i, row := range d.Rows {
		draw -= row.Weights[0]
		if draw <= 0 {
			return i, row.Values[0]
		}
	}

	panic("empty TPC-H query distribution")
}

// QGEN seeds a separate RNG for each query by advancing the stream seed once
// per query number; query execution order must not change its substitutions.
func streamParams(scale float64, seed int64) (map[string]map[string]any, error) {
	dists, err := queryDistributions()
	if err != nil {
		return nil, err
	}

	params := make(map[string]map[string]any, len(queryNames))

	initial := queryRNG(seed)
	for number, name := range queryNames {
		r := queryRNG(initial.next())
		params[name] = r.parameters(number+1, scale, dists)
	}

	return params, nil
}

// Each case follows its query's substitution rules in TPC-H §2.4.
//
//nolint:cyclop,funlen // query-specific parameter domains belong together.
func (r *queryRNG) parameters(number int, scale float64, dists map[string]queryDistribution) map[string]any {
	pick := func(name string) string {
		_, value := r.pick(dists[name])

		return value
	}
	brand := func() string { return fmt.Sprintf("Brand#%d%d", r.between(1, 5), r.between(1, 5)) }
	month := func(low, high int) string {
		index := r.between(low, high)

		return fmt.Sprintf("%04d-%02d-01", 1993+index/12, index%12+1)
	}
	year := func() string { return fmt.Sprintf("%04d-01-01", r.between(1993, 1997)) }

	var p map[string]any

	switch number {
	case 1:
		p = map[string]any{"delta": r.between(60, 120)}
	case 2:
		size := r.between(1, 50)
		words := strings.Fields(pick("p_types"))
		p = map[string]any{"size": size, "type": words[2], "region": pick("regions")}
	case 3:
		p = map[string]any{"segment": pick("msegmnt"), "date": fmt.Sprintf("1995-03-%02d", r.between(1, 31))}
	case 4:
		p = map[string]any{"date": month(0, 57)}
	case 5:
		p = map[string]any{"region": pick("regions"), "date": year()}
	case 6:
		p = map[string]any{"date": year(), "discount": float64(r.between(2, 9)) / 100, "quantity": r.between(24, 25)}
	case 7:
		first := pick("nations2")

		second := pick("nations2")
		for second == first {
			second = pick("nations2")
		}

		p = map[string]any{"nation1": first, "nation2": second}
	case 8:
		index, nation := r.pick(dists["nations2"])

		region := 0
		for _, row := range dists["nations"].Rows[:index+1] {
			region += row.Weights[0]
		}

		p = map[string]any{"nation": nation, "region": dists["regions"].Rows[region].Values[0], "type": pick("p_types")}
	case 9:
		p = map[string]any{"color": pick("colors")}
	case 10:
		p = map[string]any{"date": month(1, 24)}
	case 11:
		fraction := fmt.Sprintf("%.10f", 0.0001/scale)
		p = map[string]any{"nation": pick("nations2"), "fraction": fraction}
	case 12:
		first := pick("smode")

		second := pick("smode")
		for second == first {
			second = pick("smode")
		}

		p = map[string]any{"shipmode1": first, "shipmode2": second, "date": year()}
	case 13:
		p = map[string]any{"word1": pick("Q13a"), "word2": pick("Q13b")}
	case 14:
		p = map[string]any{"date": month(0, 59)}
	case 15:
		p = map[string]any{"date": month(0, 57)}
	case 16:
		b := brand()
		words := strings.Fields(pick("p_types"))

		p = map[string]any{"brand": b, "type_prefix": strings.Join(words[:2], " ")}
		for i, value := range r.permutation(50)[:8] {
			p[fmt.Sprintf("s%d", i+1)] = value + 1
		}
	case 17:
		p = map[string]any{"brand": brand(), "container": pick("p_cntr")}
	case 18:
		p = map[string]any{"quantity": r.between(312, 315)}
	case 19:
		p = map[string]any{
			"brand1": brand(), "brand2": brand(), "brand3": brand(),
			"q1": r.between(1, 10), "q2": r.between(10, 20), "q3": r.between(20, 30),
		}
	case 20:
		p = map[string]any{"color": pick("colors"), "date": year(), "nation": pick("nations2")}
	case 21:
		p = map[string]any{"nation": pick("nations2")}
	case 22:
		p = map[string]any{}
		for i, value := range r.permutation(25)[:7] {
			p[fmt.Sprintf("cc%d", i+1)] = strconv.Itoa(10 + value)
		}
	}

	return p
}
