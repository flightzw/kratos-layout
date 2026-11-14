package gorm

import (
	"fmt"
	"strings"
	"time"

	"go.einride.tech/aip/filtering"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FilterOption configures filter translation.
type FilterOption func(*filterOptions)

type filterOptions struct {
	fields FieldMap
	types  map[int64]*expr.Type
}

// WithFilterFieldMap maps public filter paths to storage columns.
// The map is copied when the option is created.
func WithFilterFieldMap(fields FieldMap) FilterOption {
	fields = copyFields(fields)
	return func(o *filterOptions) { o.fields = fields }
}

// ApplyFilter creates a GORM scope from a parsed, type-checked AIP filter.
// An empty filter is a no-op. Unsupported expressions set DB.Error and prevent
// query execution. Values are bound parameters; columns are quoted identifiers.
func ApplyFilter(filter filtering.Filter, options ...FilterOption) func(*gorm.DB) *gorm.DB {
	var config filterOptions
	for _, option := range options {
		option(&config)
	}
	var predicate clause.Expression
	var err error
	if filter.CheckedExpr != nil {
		config.types = filter.CheckedExpr.GetTypeMap()
		predicate, err = config.predicate(filter.CheckedExpr.GetExpr())
	}
	return func(db *gorm.DB) *gorm.DB {
		if err != nil {
			_ = db.AddError(fmt.Errorf("aip/gorm: translate filter: %w", err)) // Returned through DB.Error.
			return db
		}
		if predicate == nil {
			return db
		}
		return db.Where(predicate)
	}
}

func (o filterOptions) predicate(e *expr.Expr) (clause.Expression, error) {
	if e == nil {
		return nil, fmt.Errorf("missing expression")
	}
	call := e.GetCallExpr()
	if call == nil {
		value, err := o.operand(e)
		if err != nil {
			return nil, err
		}
		switch value.(type) {
		case bool:
			return clause.Expr{SQL: "?", Vars: []any{value}}, nil
		case clause.Column:
			if typ := o.types[e.GetId()]; typ != nil && typ.GetPrimitive() != expr.Type_BOOL {
				return nil, fmt.Errorf("non-boolean field predicate")
			}
			return clause.Eq{Column: value, Value: true}, nil
		default:
			return nil, fmt.Errorf("non-boolean predicate")
		}
	}
	if call.GetTarget() != nil {
		return nil, fmt.Errorf("receiver calls are unsupported")
	}
	args := call.GetArgs()
	switch call.GetFunction() {
	case filtering.FunctionAnd, filtering.FunctionFuzzyAnd, filtering.FunctionOr:
		if len(args) < 2 {
			return nil, fmt.Errorf("%s requires at least two operands", call.GetFunction())
		}
		predicates := make([]clause.Expression, 0, len(args))
		for _, arg := range args {
			p, err := o.predicate(arg)
			if err != nil {
				return nil, err
			}
			predicates = append(predicates, p)
		}
		if call.GetFunction() == filtering.FunctionOr {
			return clause.Or(predicates...), nil
		}
		return clause.And(predicates...), nil
	case filtering.FunctionNot:
		if len(args) != 1 {
			return nil, fmt.Errorf("NOT requires one operand")
		}
		p, err := o.predicate(args[0])
		if err != nil {
			return nil, err
		}
		return clause.Not(p), nil
	default:
		return o.comparison(call)
	}
}

func (o filterOptions) comparison(call *expr.Expr_Call) (clause.Expression, error) {
	args := call.GetArgs()
	if len(args) != 2 {
		return nil, fmt.Errorf("%s requires two operands", call.GetFunction())
	}
	left, err := o.operand(args[0])
	if err != nil {
		return nil, err
	}
	right, err := o.operand(args[1])
	if err != nil {
		return nil, err
	}
	if o.types[args[0].GetId()].GetWellKnown() == expr.Type_TIMESTAMP {
		if value, ok := right.(string); ok {
			right, err = time.Parse(time.RFC3339Nano, value)
		}
	}
	if o.types[args[1].GetId()].GetWellKnown() == expr.Type_TIMESTAMP {
		if value, ok := left.(string); ok {
			left, err = time.Parse(time.RFC3339Nano, value)
		}
	}
	if err != nil {
		return nil, err
	}
	// GORM treats the left operand as an identifier. Wrap literal operands so
	// they are bound as values, including for reversed and constant comparisons.
	leftOperand := left
	if _, ok := left.(clause.Column); !ok {
		leftOperand = clause.Expr{SQL: "?", Vars: []any{left}}
	}
	comparison := clause.Eq{Column: leftOperand, Value: right}
	switch call.GetFunction() {
	case filtering.FunctionEquals:
		return comparison, nil
	case filtering.FunctionNotEquals:
		return clause.Neq(comparison), nil
	case filtering.FunctionLessThan:
		return clause.Lt(comparison), nil
	case filtering.FunctionLessEquals:
		return clause.Lte(comparison), nil
	case filtering.FunctionGreaterThan:
		return clause.Gt(comparison), nil
	case filtering.FunctionGreaterEquals:
		return clause.Gte(comparison), nil
	case filtering.FunctionHas:
		if typ := o.types[args[0].GetId()]; typ != nil && typ.GetPrimitive() != expr.Type_STRING {
			return nil, fmt.Errorf("has supports only string containment")
		}
		value, ok := right.(string)
		if _, isColumn := left.(clause.Column); !isColumn || !ok {
			return nil, fmt.Errorf("has supports only a string column and string literal")
		}
		// Escape SQL wildcards so ':' means literal substring containment.
		value = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(value)
		return clause.Expr{SQL: "? LIKE ? ESCAPE '!'", Vars: []any{left, "%" + value + "%"}}, nil
	default:
		return nil, fmt.Errorf("unsupported function %q", call.GetFunction())
	}
}

//nolint:staticcheck // Support legacy timestamp and duration constants in AIP CheckedExpr.
func (o filterOptions) operand(e *expr.Expr) (any, error) {
	if e == nil {
		return nil, fmt.Errorf("missing operand")
	}
	if path, ok := fieldPath(e); ok {
		return column(path, o.fields)
	}
	if c := e.GetConstExpr(); c != nil {
		switch v := c.GetConstantKind().(type) {
		case *expr.Constant_BoolValue:
			return v.BoolValue, nil
		case *expr.Constant_StringValue:
			return v.StringValue, nil
		case *expr.Constant_Int64Value:
			return v.Int64Value, nil
		case *expr.Constant_Uint64Value:
			return v.Uint64Value, nil
		case *expr.Constant_DoubleValue:
			return v.DoubleValue, nil
		case *expr.Constant_DurationValue:
			if err := v.DurationValue.CheckValid(); err != nil {
				return nil, err
			}
			return v.DurationValue.AsDuration(), nil
		case *expr.Constant_TimestampValue:
			if err := v.TimestampValue.CheckValid(); err != nil {
				return nil, err
			}
			return v.TimestampValue.AsTime(), nil
		}
	}
	call := e.GetCallExpr()
	if call != nil && call.GetTarget() == nil && len(call.GetArgs()) == 1 {
		arg := call.GetArgs()[0].GetConstExpr()
		if _, ok := arg.GetConstantKind().(*expr.Constant_StringValue); ok {
			switch call.GetFunction() {
			case filtering.FunctionTimestamp:
				return time.Parse(time.RFC3339Nano, arg.GetStringValue())
			case filtering.FunctionDuration:
				return time.ParseDuration(arg.GetStringValue())
			}
		}
	}
	return nil, fmt.Errorf("unsupported operand %T", e.GetExprKind())
}

func fieldPath(e *expr.Expr) (string, bool) {
	if ident := e.GetIdentExpr(); ident != nil {
		return ident.GetName(), true
	}
	if selectExpr := e.GetSelectExpr(); selectExpr != nil && !selectExpr.GetTestOnly() {
		parent, ok := fieldPath(selectExpr.GetOperand())
		if ok {
			return parent + "." + selectExpr.GetField(), true
		}
	}
	return "", false
}
