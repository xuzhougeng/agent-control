package main

import "testing"

func TestShouldRetryWithContinueFlag_AlreadyInUse(t *testing.T) {
	if !shouldRetryWithContinueFlag("Error: Session ID abc is already in use.") {
		t.Fatal("expected already-in-use stderr to trigger continue-session retry")
	}
}
