package banking

import "testing"

// TestArmingHappensOncePerIteration pins the property the retry path depends on.
//
// The armed fault is one-shot, so the retry of an iteration that already armed one
// has to run without a fresh fault and commit — that is how a retry absorbs an
// injected failure at all. Arming per *attempt* instead of per iteration was
// measured against a real server: 2.76 arms per armed iteration, and every attempt
// failed, so no retry could recover and the failure count barely moved.
func TestArmingHappensOncePerIteration(t *testing.T) {
	everyFifth := func(workers int) *workload {
		armed := make([]uint64, workers)
		for i := range armed {
			armed[i] = neverArmed
		}

		return &workload{inject: &injectConfig{every: 5}, injectArmedFor: armed}
	}

	w := everyFifth(1)

	if !w.shouldArm(0, 5) {
		t.Fatal("the fifth iteration did not arm the injector")
	}

	if w.shouldArm(0, 5) {
		t.Error("a retry of the same iteration armed a second event: every attempt would fail and no retry could absorb the fault")
	}

	if w.shouldArm(0, 5) {
		t.Error("a third attempt of the same iteration armed yet another event")
	}

	if !w.shouldArm(0, 10) {
		t.Error("the next armed iteration did not arm")
	}

	if w.shouldArm(0, 6) {
		t.Error("an iteration that is not a multiple of --inject-every armed")
	}

	// Workers keep their own record: one worker's armed iteration must not stop
	// another worker from arming in the same iteration.
	twoWorkers := everyFifth(2)

	if !twoWorkers.shouldArm(0, 5) {
		t.Error("worker 0 did not arm")
	}

	if !twoWorkers.shouldArm(1, 5) {
		t.Error("worker 1 did not arm in the same iteration as worker 0")
	}

	// --inject-every 0 disables injection, and the portable variant never carries
	// an injector at all.
	disabled := &workload{inject: &injectConfig{every: 0}, injectArmedFor: []uint64{neverArmed}}
	if disabled.shouldArm(0, 0) {
		t.Error("--inject-every 0 must disable injection")
	}

	portable := &workload{injectArmedFor: []uint64{neverArmed}}
	if portable.shouldArm(0, 0) {
		t.Error("the portable variant must never arm an injector")
	}
}
