package gorm_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	aipgorm "github.com/flightzw/kratos-layout/pkg/aip/gorm"

	_ "modernc.org/sqlite"
)

type record struct {
	ID        int
	Name      string
	Age       int
	Score     float64
	Active    bool
	CreatedAt time.Time
	Lifetime  time.Duration
}

func database(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: "sqlite", DSN: ":memory:"}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.AutoMigrate(&record{}); err != nil {
		t.Fatal(err)
	}
	rows := []record{
		{ID: 1, Name: "alpha", Age: 20, Score: 19.5, Active: true, CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Lifetime: time.Hour},
		{ID: 2, Name: "beta", Age: 30, Score: 30.5, CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Lifetime: 2 * time.Hour},
		{ID: 3, Name: "100%_done!", Age: 40, Score: 40.5, Active: true, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Lifetime: 3 * time.Hour},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func parse(t *testing.T, input string) filtering.Filter {
	t.Helper()
	declarations, err := filtering.NewDeclarations(
		filtering.DeclareStandardFunctions(),
		filtering.DeclareFunction(filtering.FunctionFuzzyAnd, filtering.NewFunctionOverload("fuzzy_bool", filtering.TypeBool, filtering.TypeBool, filtering.TypeBool)),
		filtering.DeclareIdent("name", filtering.TypeString),
		filtering.DeclareIdent("display_name", filtering.TypeString),
		filtering.DeclareIdent("records.name", filtering.TypeString),
		filtering.DeclareIdent("age", filtering.TypeInt),
		filtering.DeclareIdent("score", filtering.TypeFloat),
		filtering.DeclareIdent("active", filtering.TypeBool),
		filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("lifetime", filtering.TypeDuration),
		filtering.DeclareIdent("tags", filtering.TypeList(filtering.TypeString)),
	)
	if err != nil {
		t.Fatal(err)
	}
	filter, err := filtering.ParseFilterString(input, declarations)
	if err != nil {
		t.Fatal(err)
	}
	return filter
}

func TestApplyFilter(t *testing.T) {
	db := database(t)
	tests := []struct {
		name   string
		filter string
		ids    []int
	}{
		{name: "empty", ids: []int{1, 2, 3}},
		{name: "equals", filter: `name = "alpha"`, ids: []int{1}},
		{name: "not equals", filter: `age != 30`, ids: []int{1, 3}},
		{name: "range", filter: `age >= 30 AND age < 40`, ids: []int{2}},
		{name: "less equals", filter: `age <= 20`, ids: []int{1}},
		{name: "greater", filter: `age > 30`, ids: []int{3}},
		{name: "fuzzy and", filter: `age >= 30 age < 40`, ids: []int{2}},
		{name: "grouping", filter: `(age = 20 OR age = 30) AND active`, ids: []int{1}},
		{name: "not", filter: `NOT (age = 20 OR age = 30)`, ids: []int{3}},
		{name: "bare bool", filter: `active`, ids: []int{1, 3}},
		{name: "double and column", filter: `score > age`, ids: []int{2, 3}},
		{name: "double literal", filter: `score > 30.0`, ids: []int{2, 3}},
		{name: "substring", filter: `name : "pha"`, ids: []int{1}},
		{name: "literal wildcards", filter: `name : "%_done!"`, ids: []int{3}},
		{name: "bound value", filter: `name = "' OR 1=1 --"`},
		{name: "qualified column", filter: `records.name = "beta"`, ids: []int{2}},
		{name: "mapped field", filter: `display_name = "alpha"`, ids: []int{1}},
		{name: "timestamp", filter: `created_at >= timestamp("2025-01-01T00:00:00Z")`, ids: []int{2, 3}},
		{name: "timestamp string", filter: `created_at >= "2025-01-01T00:00:00Z"`, ids: []int{2, 3}},
		{name: "duration", filter: `lifetime > duration("1h")`, ids: []int{2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rows []record
			err := db.Scopes(aipgorm.ApplyFilter(parse(t, tt.filter), aipgorm.WithFilterFieldMap(aipgorm.FieldMap{"display_name": "name"}))).Order("id").Find(&rows).Error
			if err != nil {
				t.Fatal(err)
			}
			var ids []int
			for _, row := range rows {
				ids = append(ids, row.ID)
			}
			if !reflect.DeepEqual(ids, tt.ids) {
				t.Fatalf("ids = %v, want %v", ids, tt.ids)
			}
		})
	}
}

func TestApplyOrderBy(t *testing.T) {
	db := database(t)
	fields := aipgorm.FieldMap{"display_name": "records.name"}
	option := aipgorm.WithOrderByFieldMap(fields)
	fields["display_name"] = "invalid"
	var order ordering.OrderBy
	if err := order.UnmarshalString("active desc, display_name desc"); err != nil {
		t.Fatal(err)
	}
	var rows []record
	if err := db.Scopes(aipgorm.ApplyOrderBy(order, option)).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	ids := []int{rows[0].ID, rows[1].ID, rows[2].ID}
	if !reflect.DeepEqual(ids, []int{1, 3, 2}) {
		t.Fatalf("ids = %v", ids)
	}
	if err := db.Scopes(aipgorm.ApplyOrderBy(ordering.OrderBy{})).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
}

func TestConstantAndReversedPredicates(t *testing.T) {
	db := database(t)
	reversed := parse(t, `age > 30`)
	args := reversed.CheckedExpr.Expr.GetCallExpr().Args
	args[0], args[1] = args[1], args[0]
	boolean := func(value bool) filtering.Filter {
		return filtering.Filter{CheckedExpr: &expr.CheckedExpr{Expr: &expr.Expr{
			ExprKind: &expr.Expr_ConstExpr{ConstExpr: &expr.Constant{
				ConstantKind: &expr.Constant_BoolValue{BoolValue: value},
			}},
		}}}
	}
	tests := []struct {
		name   string
		filter filtering.Filter
		count  int64
	}{
		{name: "true", filter: boolean(true), count: 3},
		{name: "false", filter: boolean(false)},
		{name: "reversed", filter: reversed, count: 1},
		{name: "constant comparison", filter: parse(t, `20 < 30`), count: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var count int64
			if err := db.Model(&record{}).Scopes(aipgorm.ApplyFilter(tt.filter)).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != tt.count {
				t.Fatalf("count = %d, want %d", count, tt.count)
			}
		})
	}
}

func TestTranslationErrorsPreventExecution(t *testing.T) {
	db := database(t)
	invalidTimestamp := parse(t, `created_at = timestamp("2025-01-01T00:00:00Z")`)
	invalidTimestamp.CheckedExpr.Expr.GetCallExpr().Args[1].GetCallExpr().Args[0].GetConstExpr().ConstantKind = &expr.Constant_StringValue{StringValue: "invalid"}
	invalidDuration := parse(t, `lifetime = duration("1h")`)
	invalidDuration.CheckedExpr.Expr.GetCallExpr().Args[1].GetCallExpr().Args[0].GetConstExpr().ConstantKind = &expr.Constant_StringValue{StringValue: "invalid"}
	tests := []struct {
		name  string
		scope func(*gorm.DB) *gorm.DB
	}{
		{name: "missing expression", scope: aipgorm.ApplyFilter(filtering.Filter{CheckedExpr: &expr.CheckedExpr{}})},
		{name: "unsupported has", scope: aipgorm.ApplyFilter(parse(t, `tags : "foo"`))},
		{name: "invalid timestamp", scope: aipgorm.ApplyFilter(invalidTimestamp)},
		{name: "invalid duration", scope: aipgorm.ApplyFilter(invalidDuration)},
		{name: "bad mapping", scope: aipgorm.ApplyFilter(parse(t, `name = "alpha"`), aipgorm.WithFilterFieldMap(aipgorm.FieldMap{"name": "name; DROP TABLE records"}))},
		{name: "invalid order", scope: aipgorm.ApplyOrderBy(ordering.OrderBy{Fields: []ordering.Field{{Path: "id"}, {Path: "id; DROP TABLE records"}}})},
		{name: "empty path", scope: aipgorm.ApplyOrderBy(ordering.OrderBy{Fields: []ordering.Field{{Path: "records..id"}}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rows []record
			result := db.Session(&gorm.Session{DryRun: true}).Scopes(tt.scope).Find(&rows)
			if result.Error == nil || !strings.Contains(result.Error.Error(), "aip/gorm:") {
				t.Fatalf("expected translation error, got %v", result.Error)
			}
			if result.Statement.SQL.Len() != 0 {
				t.Fatalf("failed translation produced SQL: %s", result.Statement.SQL.String())
			}
		})
	}
}
