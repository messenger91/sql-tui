package db

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
)

func TestIsNoResultSet(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"driver skip", driver.ErrSkip, true},
		{"wrapped driver skip", fmt.Errorf("query: %w", driver.ErrSkip), true},
		{"no result set", errors.New("sql: statement has no result set"), true},
		{"does not return rows", errors.New("statement does not return rows"), true},
		{"doesn't return rows", errors.New("statement doesn't return rows"), true},
		{"no rows is not enough", errors.New("sql: no rows in result set"), false},
		{"unrelated error", errors.New("syntax error"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNoResultSet(tc.err); got != tc.want {
				t.Fatalf("isNoResultSet(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
