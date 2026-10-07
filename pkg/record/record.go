// Package record provides structured, deterministic operation recordings for workload tests.
package record

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sync"
)

// Scope identifies an operation stream; order is preserved within each stream.
type Scope struct {
	Database  string `json:"database"`
	Step      string `json:"step"`
	Worker    int    `json:"worker"`
	Iteration uint64 `json:"iteration"`
}
type scopeKey struct{}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

func ContextScope(ctx context.Context) Scope {
	value, _ := ctx.Value(scopeKey{}).(Scope)

	return value
}

// Value retains the Go value type and its JSON representation, including NULL.
type Value struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"value"`
}

func Encode(value any) (Value, error) {
	if value == nil {
		return Value{Type: "null", Data: json.RawMessage("null")}, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return Value{}, fmt.Errorf("record value: %w", err)
	}

	return Value{Type: reflect.TypeOf(value).String(), Data: data}, nil
}

// Operation is one query, transaction boundary or insertion.
type Operation struct {
	Scope       Scope            `json:"scope"`
	Sequence    uint64           `json:"sequence"`
	Kind        string           `json:"kind"`
	Transaction uint64           `json:"transaction,omitempty"`
	SQL         string           `json:"sql,omitempty"`
	Arguments   map[string]Value `json:"arguments,omitempty"`
	Isolation   string           `json:"isolation,omitempty"`
	Table       string           `json:"table,omitempty"`
	Columns     []string         `json:"columns,omitempty"`
	Method      string           `json:"method,omitempty"`
	Rows        int64            `json:"rows,omitempty"`
	Sample      [][]Value        `json:"sample,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// Response supplies an explicit answer to a query. Missing answers have no rows.
type Response struct {
	Columns []string
	Rows    [][]any
	Err     error
}

// Recorder is safe for concurrent workers. Zero value is ready for use.
// SampleRows bounds rows retained per insert; zero records counts only.
type Recorder struct {
	SampleRows   int
	mu           sync.Mutex
	operations   []Operation
	sequences    map[Scope]uint64
	transactions map[Scope]uint64
	responses    map[string][]Response
}

// Reply queues an answer for matching SQL; answers are consumed in call order.
// Use separate recorders when concurrent identical queries need distinct answers.
func (r *Recorder) Reply(sql string, response Response) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.responses == nil {
		r.responses = map[string][]Response{}
	}

	response.Columns = slices.Clone(response.Columns)
	response.Rows = copyRows(response.Rows)
	r.responses[sql] = append(r.responses[sql], response)
}

func (r *Recorder) Answer(sql string) Response {
	r.mu.Lock()
	defer r.mu.Unlock()

	values := r.responses[sql]
	if len(values) == 0 {
		return Response{}
	}

	r.responses[sql] = values[1:]

	return values[0]
}

func copyRows(rows [][]any) [][]any {
	out := make([][]any, len(rows))
	for i, row := range rows {
		out[i] = slices.Clone(row)
		for j, value := range out[i] {
			if bytes, ok := value.([]byte); ok {
				out[i][j] = slices.Clone(bytes)
			}
		}
	}

	return out
}

//nolint:gocritic // records are copied at publication time.
func (r *Recorder) Add(operation Operation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.sequences == nil {
		r.sequences = map[Scope]uint64{}
	}

	stream := operation.Scope
	stream.Database = ""
	operation.Sequence = r.sequences[stream]
	r.sequences[stream]++
	r.operations = append(r.operations, cloneOperation(&operation))
}

func (r *Recorder) Begin(scope Scope) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.transactions == nil {
		r.transactions = map[Scope]uint64{}
	}

	r.transactions[scope]++

	return r.transactions[scope]
}

// Operations returns an owned snapshot sorted by scope, retaining local order.
func (r *Recorder) Operations() []Operation {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Operation, len(r.operations))
	for i := range r.operations {
		out[i] = cloneOperation(&r.operations[i])
	}

	slices.SortFunc(out, func(a, b Operation) int {
		if a.Scope.Step != b.Scope.Step {
			return compare(a.Scope.Step, b.Scope.Step)
		}

		if a.Scope.Worker != b.Scope.Worker {
			return compare(a.Scope.Worker, b.Scope.Worker)
		}

		if a.Scope.Iteration != b.Scope.Iteration {
			return compare(a.Scope.Iteration, b.Scope.Iteration)
		}

		return compare(a.Sequence, b.Sequence)
	})

	return out
}

func cloneOperation(original *Operation) Operation {
	value := *original
	value.Columns = slices.Clone(value.Columns)

	arguments := make(map[string]Value, len(value.Arguments))
	for name, argument := range value.Arguments {
		argument.Data = slices.Clone(argument.Data)
		arguments[name] = argument
	}

	value.Arguments = arguments

	sample := make([][]Value, len(value.Sample))
	for i, row := range value.Sample {
		sample[i] = slices.Clone(row)
		for j := range sample[i] {
			sample[i][j].Data = slices.Clone(sample[i][j].Data)
		}
	}

	value.Sample = sample

	return value
}

func compare[T ~string | ~int | ~uint64](a, b T) int {
	if a < b {
		return -1
	}

	if a > b {
		return 1
	}

	return 0
}

// WriteTo emits one versioned JSON document, without runtime IDs or timings.
func (r *Recorder) WriteTo(output io.Writer) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")

	return encoder.Encode(struct {
		Schema     int         `json:"schema"`
		Operations []Operation `json:"operations"`
	}{1, r.Operations()})
}
