package aip

import (
	"github.com/go-kratos/kratos/v3/errors"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
)

var DefaultPageSize int32 = 20

type Request interface {
	GetPage() int32
	GetPageSize() int32
	GetSort() string
	GetOrder() string
}

func ParsePaginationOptions(req Request) ([]ListOption, error) {
	page, size := req.GetPage(), req.GetPageSize()
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = DefaultPageSize
	}
	offset := int64(page-1) * int64(size)
	sort, direction := req.GetSort(), req.GetOrder()
	if sort == "" {
		sort = "id"
	}
	if direction == "" {
		direction = "asc"
	}
	var orderBy ordering.OrderBy
	if err := orderBy.UnmarshalString(sort + " " + direction); err != nil {
		return nil, errors.BadRequest("SORT_RULE_INVALID", "invalid sort rule").WithCause(err)
	}
	return []ListOption{
		ListOffset(int(offset)),
		ListLimit(int(size)),
		ListOrderBy(orderBy),
	}, nil
}

func ParseFilter(req filtering.Request, decl *filtering.Declarations) (filtering.Filter, error) {
	filter, err := filtering.ParseFilterString(req.GetFilter(), decl)
	if err != nil {
		return filter, errors.BadRequest("FILTER_RULE_INVALID", "invalid filter string").WithCause(err)
	}
	return filter, nil
}
