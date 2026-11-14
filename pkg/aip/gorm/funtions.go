package gorm

import (
	"fmt"

	"github.com/flightzw/kratos-layout/pkg/aip"
	"gorm.io/gorm"
)

// FindByPage executes a paginated query with the given options and returns the total number of records matching the filter.
func FindByPage(db *gorm.DB, dest any, opts ...aip.ListOption) (total int64, err error) {
	options := aip.ListOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	if options.Limit <= 0 {
		options.Limit = int(aip.DefaultPageSize)
	}
	db = db.Scopes(ApplyFilter(options.Filter), ApplyOrderBy(options.OrderBy))
	if err := db.Offset(options.Offset).Limit(options.Limit).Find(dest).Error; err != nil {
		return 0, fmt.Errorf("db.Find: %w", err)
	}
	if err := db.Offset(-1).Limit(-1).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("db.Count: %w", err)
	}
	return total, nil
}
