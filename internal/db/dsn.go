package db

import (
	"fmt"
	"net/url"
	"strings"
)

// OptionSchema requests a default schema (Postgres search_path) or database
// (MySQL USE) after connecting.
const OptionSchema = "schema"

// ParseDSN accepts URL-style DSNs in addition to native driver DSNs:
//
//	mysql://user:pass@host:3306/db?parseTime=true
//	postgres://... (or postgresql://...)
//	sqlite:///path/to.db, sqlite::memory:
func ParseDSN(driver Driver, dsn string) (string, error) {
	trimmed := strings.TrimSpace(dsn)
	switch driver {
	case DriverMySQL:
		if strings.HasPrefix(trimmed, "mysql://") {
			return parseMySQLURL(trimmed)
		}
	case DriverPostgres:
		if strings.HasPrefix(trimmed, "postgresql://") {
			return "postgres://" + strings.TrimPrefix(trimmed, "postgresql://"), nil
		}
	case DriverSQLite:
		if strings.HasPrefix(trimmed, "sqlite://") {
			return strings.TrimPrefix(trimmed, "sqlite://"), nil
		}
		if strings.HasPrefix(trimmed, "sqlite:") {
			return strings.TrimPrefix(trimmed, "sqlite:"), nil
		}
	}
	return dsn, nil
}

func parseMySQLURL(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("db: parse mysql url: %w", err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("db: mysql url is missing a host")
	}
	var builder strings.Builder
	if parsed.User != nil {
		builder.WriteString(parsed.User.Username())
		if password, ok := parsed.User.Password(); ok {
			builder.WriteString(":")
			builder.WriteString(password)
		}
		builder.WriteString("@")
	}
	builder.WriteString("tcp(")
	builder.WriteString(parsed.Host)
	builder.WriteString(")/")
	builder.WriteString(strings.TrimPrefix(parsed.Path, "/"))
	if parsed.RawQuery != "" {
		builder.WriteString("?")
		builder.WriteString(parsed.RawQuery)
	}
	return builder.String(), nil
}
