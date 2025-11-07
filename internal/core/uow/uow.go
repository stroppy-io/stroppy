package uow

// UnitOfWork tracks rollback actions for a sequence of steps.
type UnitOfWork struct {
	rollbacks []func()
}

// Defer registers a rollback action, executed in LIFO order on rollback.
func (u *UnitOfWork) Defer(f func()) {
	if f == nil {
		return
	}
	u.rollbacks = append(u.rollbacks, f)
}

// Commit clears rollback actions so nothing is reverted.
func (u *UnitOfWork) Commit() {
	u.rollbacks = nil
}

// Rollback runs all registered rollback actions in reverse order.
func (u *UnitOfWork) Rollback() {
	for i := len(u.rollbacks) - 1; i >= 0; i-- {
		// protect against panics in rollback callbacks
		func(fn func()) {
			defer func() { _ = recover() }()
			fn()
		}(u.rollbacks[i])
	}
	u.rollbacks = nil
}

// With runs fn as a unit of work.
// - If fn returns an error, all rollback callbacks run.
// - If fn returns nil, rollbacks are discarded (committed).
func With(fn func(u *UnitOfWork) error) (err error) {
	u := &UnitOfWork{}

	defer func() {
		if err != nil {
			u.Rollback()
		} else {
			u.Commit()
		}
	}()

	err = fn(u)
	return
}
