package bench

// Handler is the unit of work an executor drives. Its three methods correspond
// to the RFC 0001 §6 allocation phases:
//
//   - Init runs once per VU (per worker for Pool) in the plan phase: allocate
//     everything the hot path will touch and stash it in vu.Local.
//   - Iter runs in the hot phase and must be allocation-free in steady state.
//     Its returned error is a value, classified by the executor's [ErrorMode]
//     (or an explicit [Fail]/[Abort] root error); it is never thrown.
//   - Close runs once per VU (per worker for Pool) at teardown, even when the
//     run is aborted or its context is canceled.
//
// A single Handler value is shared across all VUs of an executor, so it must
// carry no mutable per-VU state: per-VU state belongs in [VU.Local], set by Init
// and read by Iter/Close. This keeps the Handler itself immutable and safe to
// share.
type Handler interface {
	Init(vu *VU) error
	Iter(vu *VU) error
	Close(vu *VU) error
}

// FuncOnce adapts a plain func(*VU) error into a [Handler] with no Init/Close,
// for trivial run-once bodies (DDL, a validation query) where the phase
// structure would be ceremony. Init and Close are no-ops; the func is the Iter.
func FuncOnce(fn func(vu *VU) error) Handler { return funcOnce{fn: fn} }

type funcOnce struct{ fn func(vu *VU) error }

func (f funcOnce) Init(*VU) error    { return nil }
func (f funcOnce) Iter(vu *VU) error { return f.fn(vu) }
func (f funcOnce) Close(*VU) error   { return nil }
