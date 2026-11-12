package report

// Pool keeps unsupported observations distinct from measured zero contention.
type Pool struct {
	Step           string   `json:"step"`
	Execution      int      `json:"execution"`
	Database       string   `json:"database"`
	Driver         string   `json:"driver"`
	Backend        string   `json:"backend,omitempty"`
	Scope          string   `json:"scope,omitempty"`
	Supported      bool     `json:"supported"`
	Reason         string   `json:"reason,omitempty"`
	Open           *int64   `json:"open,omitempty"`
	InUse          *int64   `json:"in_use,omitempty"`
	Idle           *int64   `json:"idle,omitempty"`
	MaxConnections *int64   `json:"max_connections,omitempty"`
	WaitCount      *float64 `json:"wait_count,omitempty"`
	WaitSeconds    *float64 `json:"wait_seconds,omitempty"`
	Canceled       *float64 `json:"canceled_acquires,omitempty"`
}
