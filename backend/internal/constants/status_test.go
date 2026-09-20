package constants

import "testing"

func TestPressUnitTransitionGraph(t *testing.T) {
	if !CanTransition(PressUnitTransitions, "ready", "setup") {
		t.Fatalf("expected ready -> setup transition to be allowed")
	}
	if CanTransition(PressUnitTransitions, "ready", "unknown") {
		t.Fatal("unknown status must never be accepted")
	}
}

func TestColorProofDriftPendingTransitions(t *testing.T) {
	// Over-limit proofs are diverted to review_pending, which only supports
	// reviewer re-gating and accept/reject decisions.
	for _, target := range []string{"review", "accepted", "rejected"} {
		if !CanTransition(ColorProofTransitions, "review_pending", target) {
			t.Fatalf("review_pending -> %s must be gated by the service, not the state machine", target)
		}
	}
	if !CanTransition(ColorProofTransitions, "review", "review_pending") {
		t.Fatal("review must be able to divert an over-limit proof into review_pending")
	}
	if CanTransition(ColorProofTransitions, "captured", "accepted") {
		t.Fatal("a freshly captured proof must never bypass the review gate")
	}
	if CanTransition(ColorProofTransitions, "review_pending", "captured") {
		t.Fatal("a drift-blocked proof must not silently return to captured without review")
	}
}
