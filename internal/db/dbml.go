package db

import (
	"fmt"
	"sort"
	"strings"
)

// DBML renders the schema as Database Markup Language (dbdiagram.io style).
// Views are included as commented tables.
func DBML(tables []TableMeta) string {
	if len(tables) == 0 {
		return ""
	}
	sorted := append([]TableMeta(nil), tables...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var body strings.Builder
	var refs strings.Builder

	for _, table := range sorted {
		if table.Kind == TableKindView {
			body.WriteString("// view\n")
		}
		fmt.Fprintf(&body, "Table %s {\n", table.Name)
		for _, column := range table.Columns {
			settings := make([]string, 0, 2)
			if column.IsPrimaryKey {
				settings = append(settings, "pk")
			}
			if !column.Nullable {
				settings = append(settings, "not null")
			}
			line := fmt.Sprintf("  %s %s", column.Name, column.Type)
			if len(settings) > 0 {
				line += " [" + strings.Join(settings, ", ") + "]"
			}
			body.WriteString(line + "\n")
		}
		body.WriteString("}\n\n")

		for _, fk := range table.ForeignKeys {
			fmt.Fprintf(&refs, "Ref: %s.%s > %s.%s\n", table.Name, fk.Column, fk.RefTable, fk.RefColumn)
		}
	}

	result := strings.TrimRight(body.String(), "\n")
	if refs.Len() > 0 {
		result += "\n\n" + strings.TrimRight(refs.String(), "\n")
	}
	return result + "\n"
}
