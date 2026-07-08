package bench

import (
	"encoding/json"
	"io"
)

// probeDoc is the -probe JSON: a machine-stable description of a test used as
// the CLI contract. Field names are fixed; slices are emitted in declaration
// order.
type probeDoc struct {
	Name      string          `json:"name"`
	Seed      uint64          `json:"seed"`
	Variant   string          `json:"variant"`
	Params    []ParamSchema   `json:"params"`
	Drivers   []probeDriver   `json:"drivers"`
	QuerySets []probeQuerySet `json:"querySets,omitempty"`
	Variants  []string        `json:"variants,omitempty"`
	Steps     []probeStep     `json:"steps"`
}

// probeQuerySet is one query-set's resolution as probe reports it: the name a
// user would override, the active driver kind the resolution used, the
// provenance of the winning source, and (when resolution failed) the error a
// user acts on. Override names follow the convention STROPPY_QUERIES_<NAME>
// (upper-cased name).
type probeQuerySet struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	Override string `json:"override"`
	Error    string `json:"error,omitempty"`
}

type probeDriver struct {
	Name     string            `json:"name"`
	Kind     string            `json:"kind"`
	URL      string            `json:"url"`
	Mode     string            `json:"mode"`
	MinConns int32             `json:"minConns,omitempty"`
	MaxConns int32             `json:"maxConns,omitempty"`
	Sources  map[string]string `json:"sources,omitempty"`
	Native   map[string]any    `json:"native,omitempty"`
}

type probeExec struct {
	Kind     string   `json:"kind"`
	VUs      int      `json:"vus,omitempty"`
	Workers  int      `json:"workers,omitempty"`
	Items    []string `json:"items,omitempty"`
	Duration string   `json:"duration,omitempty"`
	Iters    uint64   `json:"iters,omitempty"`
	Rate     float64  `json:"rate,omitempty"`
}

type probeStep struct {
	Name      string    `json:"name"`
	Executor  probeExec `json:"executor"`
	After     []string  `json:"after,omitempty"`
	AfterAny  []string  `json:"afterAny,omitempty"`
	OnFailure []string  `json:"onFailure,omitempty"`
	If        bool      `json:"if"`
	OnErr     string    `json:"onErr"`
	Uses      string    `json:"uses"`
	UsesSlot  int       `json:"usesSlot"`
	Skippable bool      `json:"skippable,omitempty"`
}

// buildProbe assembles the probe description from the test, its active steps,
// the resolved slots, and the declarations captured during Define. The run
// carries the query-set resolutions recorded as each d.Queries ran, so probe
// reports the exact set an override would target; d carries the declared
// variants.
func buildProbe(t *Test, steps []*StepDef, seed uint64, schema []ParamSchema, slots []slotSpec, run *Run, d *Def, chosenVariant string) probeDoc {
	doc := probeDoc{Name: t.Name, Seed: seed, Variant: chosenVariant, Params: schema}
	for _, s := range slots {
		pd := probeDriver{
			Name:     s.name,
			Kind:     s.kind,
			URL:      s.spec.URL,
			Mode:     s.spec.Mode.String(),
			MinConns: s.spec.MinConns,
			MaxConns: s.spec.MaxConns,
			Native:   s.spec.Native,
		}
		if len(s.spec.Sources) > 0 {
			pd.Sources = make(map[string]string, len(s.spec.Sources))
			for k, v := range s.spec.Sources {
				pd.Sources[k] = v.String()
			}
		}
		doc.Drivers = append(doc.Drivers, pd)
	}
	if run != nil {
		kind := run.activeKind()
		for _, name := range run.qOrder {
			rs := run.qset[name]
			pqs := probeQuerySet{
				Name:     name,
				Kind:     kind,
				Source:   rs.source,
				Override: queryOverrideEnv(name),
			}
			if rs.err != nil {
				pqs.Error = rs.err.Error()
			}
			doc.QuerySets = append(doc.QuerySets, pqs)
		}
	}
	if d != nil {
		for _, v := range d.variants {
			doc.Variants = append(doc.Variants, v.name)
		}
	}
	for _, sd := range steps {
		usesSlot, _ := resolveUses(sd, slots)
		doc.Steps = append(doc.Steps, probeStep{
			Name:      sd.name,
			Executor:  execProbe(sd),
			After:     sd.after,
			AfterAny:  sd.afterAny,
			OnFailure: sd.onFailure,
			If:        sd.ifPred != nil,
			OnErr:     sd.onErr.String(),
			Uses:      sd.uses,
			UsesSlot:  usesSlot,
			Skippable: sd.skippable,
		})
	}
	return doc
}

// execProbe renders a step's executor policy and its parameters.
func execProbe(sd *StepDef) probeExec {
	p := probeExec{Kind: sd.kind.String()}
	switch sd.kind {
	case kindPool:
		p.Workers, p.Items = sd.workers, sd.items
	case kindClosed:
		p.VUs, p.Iters = sd.vus, sd.iters
		if sd.dur > 0 {
			p.Duration = sd.dur.String()
		}
	case kindOpen:
		p.VUs, p.Rate = sd.vus, sd.rate
		if sd.dur > 0 {
			p.Duration = sd.dur.String()
		}
	}
	return p
}

// writeProbe marshals the probe description to w as indented JSON.
func writeProbe(w io.Writer, doc probeDoc) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
