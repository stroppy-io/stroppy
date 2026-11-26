package report

// Progress separates a heuristic completion estimate from execution outcome.
type Progress struct {
	Ratio              float64          `json:"ratio"`
	Status             string           `json:"status"`
	Method             string           `json:"method"`
	Step               string           `json:"step,omitempty"`
	ElapsedSeconds     float64          `json:"elapsed_seconds"`
	StepElapsedSeconds float64          `json:"step_elapsed_seconds"`
	Sources            []ProgressSource `json:"sources,omitempty"`
}

// ProgressSource retains native units; sources are not additive across units.
type ProgressSource struct {
	Step           string   `json:"step"`
	Worker         int      `json:"worker"`
	Activity       string   `json:"activity,omitempty"`
	Unit           string   `json:"unit"`
	Current        float64  `json:"current"`
	Total          *float64 `json:"total,omitempty"`
	ElapsedSeconds float64  `json:"elapsed_seconds"`
}
