package db

import (
	"context"
	"strings"
	"testing"
)

func TestParseDSN(t *testing.T) {
	tests := []struct {
		name    string
		driver  Driver
		dsn     string
		want    string
		wantErr bool
	}{
		{
			name: "mysql url", driver: DriverMySQL,
			dsn:  "mysql://user:pass@127.0.0.1:3306/db?parseTime=true",
			want: "user:pass@tcp(127.0.0.1:3306)/db?parseTime=true",
		},
		{
			name: "mysql url without credentials", driver: DriverMySQL,
			dsn:  "mysql://127.0.0.1:3306/db",
			want: "tcp(127.0.0.1:3306)/db",
		},
		{
			name: "mysql url missing host", driver: DriverMySQL,
			dsn:     "mysql:///db",
			wantErr: true,
		},
		{
			name: "mysql native", driver: DriverMySQL,
			dsn:  "user:pass@tcp(host:3306)/db",
			want: "user:pass@tcp(host:3306)/db",
		},
		{
			name: "postgres url", driver: DriverPostgres,
			dsn:  "postgres://u:p@host:5432/db?sslmode=disable",
			want: "postgres://u:p@host:5432/db?sslmode=disable",
		},
		{
			name: "postgresql url", driver: DriverPostgres,
			dsn:  "postgresql://u:p@host/db",
			want: "postgres://u:p@host/db",
		},
		{
			name: "sqlite path", driver: DriverSQLite,
			dsn:  "sqlite:///tmp/a.db",
			want: "/tmp/a.db",
		},
		{
			name: "sqlite memory", driver: DriverSQLite,
			dsn:  "sqlite::memory:",
			want: ":memory:",
		},
		{
			name: "sqlite native", driver: DriverSQLite,
			dsn:  ":memory:",
			want: ":memory:",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDSN(tc.driver, tc.dsn)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDSN: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseDSN = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConnectRejectsSQLiteSchemaOption(t *testing.T) {
	_, err := Connect(context.Background(), Connection{
		ID: "x", Driver: DriverSQLite, DSN: ":memory:",
		Options: map[string]string{OptionSchema: "main"},
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err = %v", err)
	}
}
