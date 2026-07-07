package bench

import (
	"encoding/json"
	"io"
)

// probeDoc is the -probe JSON: a machine-stable description of a test used as
// the CLI contract. Field names are fixed; slices are emitted in declaration
// order.
type probeDoc struct {
	Name      string           `json:"name"`
	Seed      uint64           `json:"seed"`
	Options   []OptionSchema   `json:"options"`
	Drivers   []probeDriver    `json:"drivers"`
	QuerySets []probeQuerySet  `json:"querySets,omitempty"`
	Steps     []probeStep      `json:"steps"`
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
	Name string `json:"name"`
	Kind string `json:"kind"`
	URL  string `json:"url"`
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
	Retry     int       `json:"retryMaxAttempts"`
	Uses      string    `json:"uses"`
	UsesSlot  int       `json:"usesSlot"`
}

// buildProbe assembles the probe description from the test, its built steps and
// the resolved slots. The run carries the query-set resolutions recorded while
// Build ran (each [Run.Queries] call appended one), so probe reports the exact
// set an override would target.
func buildProbe(t *Test, steps []*StepDef, seed uint64, schema []OptionSchema, slots []slotSpec, run *Run) probeDoc {
	doc := probeDoc{Name: t.Name, Seed: seed, Options: schema}
	for _, s := range slots {
		doc.Drivers = append(doc.Drivers, probeDriver{Name: s.name, Kind: s.kind, URL: s.url})
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
			Retry:     sd.retry.MaxAttempts,
			Uses:      sd.uses,
			UsesSlot:  usesSlot,
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
