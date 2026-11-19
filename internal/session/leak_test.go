package session

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces that runners and forwarder helpers leave no goroutines
// behind (the app's forwarders live in package app; this covers the runner).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
