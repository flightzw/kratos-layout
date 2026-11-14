package gorm

import (
	"fmt"

	"go.einride.tech/aip/ordering"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrderByOption configures order translation.
type OrderByOption func(*orderByOptions)

type orderByOptions struct {
	fields FieldMap
}

// WithOrderByFieldMap maps public ordering paths to storage columns.
// The map is copied when the option is created.
func WithOrderByFieldMap(fields FieldMap) OrderByOption {
	fields = copyFields(fields)
	return func(o *orderByOptions) { o.fields = fields }
}

// ApplyOrderBy creates a GORM scope preserving field order and direction.
// An empty order is a no-op. Invalid paths are reported through DB.Error.
func ApplyOrderBy(orderBy ordering.OrderBy, options ...OrderByOption) func(*gorm.DB) *gorm.DB {
	var config orderByOptions
	for _, option := range options {
		option(&config)
	}
	columns := make([]clause.OrderByColumn, 0, len(orderBy.Fields))
	var translationErr error
	for _, field := range orderBy.Fields {
		c, err := column(field.Path, config.fields)
		if err != nil {
			translationErr = fmt.Errorf("aip/gorm: translate order: %w", err)
			break
		}
		columns = append(columns, clause.OrderByColumn{Column: c, Desc: field.Desc})
	}
	return func(db *gorm.DB) *gorm.DB {
		if translationErr != nil {
			_ = db.AddError(translationErr) // Returned through DB.Error.
			return db
		}
		if len(columns) == 0 {
			return db
		}
		return db.Order(clause.OrderBy{Columns: columns})
	}
}
