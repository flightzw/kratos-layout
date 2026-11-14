// Package gorm adapts parsed AIP filters and ordering to GORM scopes.
package gorm

import (
	"fmt"
	"maps"
	"strings"
	"unicode"

	"gorm.io/gorm/clause"
)

// FieldMap maps public field paths to database columns. Unmapped paths retain
// their names. It is not an allowlist; validate public fields before querying.
type FieldMap map[string]string

func column(path string, fields FieldMap) (clause.Column, error) {
	if mapped, ok := fields[path]; ok {
		path = mapped
	}
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if part == "" {
			return clause.Column{}, fmt.Errorf("invalid column path %q", path)
		}
		for i, r := range part {
			if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
				return clause.Column{}, fmt.Errorf("invalid column path %q", path)
			}
		}
	}
	c := clause.Column{Name: parts[len(parts)-1]}
	if len(parts) > 1 {
		c.Table = strings.Join(parts[:len(parts)-1], ".")
	}
	return c, nil
}

func copyFields(fields FieldMap) FieldMap {
	return maps.Clone(fields)
}
