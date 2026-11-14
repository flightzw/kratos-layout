package gorm

import "errors"

// ErrInvalidQuery identifies unsupported or malformed query options before SQL execution.
var ErrInvalidQuery = errors.New("aip/gorm: invalid query")
