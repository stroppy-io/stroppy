package stats

import "time"

type Query struct {
	Elapsed time.Duration `json:"elapsed_nanos"`
}
