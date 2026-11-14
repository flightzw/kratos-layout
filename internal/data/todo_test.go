package data

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/flightzw/kratos-layout/api/commons/errno"
	"github.com/flightzw/kratos-layout/internal/biz"
	"github.com/flightzw/kratos-layout/internal/data/model"
	"github.com/flightzw/kratos-layout/internal/data/query"
	"github.com/flightzw/kratos-layout/pkg/aip"

	_ "modernc.org/sqlite"
)

func newTestTodoRepo(t *testing.T) (biz.TodoRepo, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: "sqlite", DSN: ":memory:"}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := db.AutoMigrate(&model.Todo{}); err != nil {
		t.Fatal(err)
	}
	return NewTodoRepo(&Data{db: db.Debug(), query: query.Use(db)}), db
}

func seedTodo(t *testing.T, db *gorm.DB, id int32, name string, deleted bool) *model.Todo {
	t.Helper()
	created := time.Date(2026, 1, int(id), 3, 4, 5, 0, time.UTC)
	record := &model.Todo{ID: id, Name: name, CreatedAt: created, UpdatedAt: created.Add(time.Hour)}
	if deleted {
		record.DeletedAt = gorm.DeletedAt{Time: created.Add(2 * time.Hour), Valid: true}
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	return record
}

func parseTodoFilter(t *testing.T, text string) filtering.Filter {
	t.Helper()
	declarations, err := filtering.NewDeclarations(
		filtering.DeclareStandardFunctions(),
		filtering.DeclareIdent("id", filtering.TypeInt),
		filtering.DeclareIdent("name", filtering.TypeString),
		filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("updated_at", filtering.TypeTimestamp),
	)
	if err != nil {
		t.Fatal(err)
	}
	filter, err := filtering.ParseFilterString(text, declarations)
	if err != nil {
		t.Fatal(err)
	}
	return filter
}

func TestCreateTodo(t *testing.T) {
	repo, db := newTestTodoRepo(t)
	ctx := context.Background()
	first, err := repo.CreateTodo(ctx, &biz.Todo{Name: "first"})
	if err != nil || first <= 0 {
		t.Fatalf("ID=%d error=%v", first, err)
	}
	second, err := repo.CreateTodo(ctx, &biz.Todo{Name: "second"})
	if err != nil || second <= first {
		t.Fatalf("second ID=%d first=%d error=%v", second, first, err)
	}
	var record model.Todo
	if err := db.First(&record, first).Error; err != nil {
		t.Fatal(err)
	}
	if record.Name != "first" || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.DeletedAt.Valid {
		t.Fatalf("stored=%+v", record)
	}
	if _, err := repo.CreateTodo(ctx, &biz.Todo{ID: first, Name: "duplicate"}); !errno.IsReasonUnspecified(err) {
		t.Fatalf("duplicate ID error=%v", err)
	}
}

func TestGetTodo(t *testing.T) {
	tests := []struct {
		name          string
		seed, deleted bool
	}{
		{name: "existing", seed: true}, {name: "missing"}, {name: "soft deleted", seed: true, deleted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db := newTestTodoRepo(t)
			var stored *model.Todo
			if tt.seed {
				stored = seedTodo(t, db, 1, "test", tt.deleted)
			}
			todo, err := repo.GetTodo(context.Background(), 1)
			if !tt.seed || tt.deleted {
				if todo != nil || !errno.IsTodoNotFound(err) {
					t.Fatalf("todo=%+v error=%v", todo, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if todo.ID != stored.ID || todo.Name != stored.Name || !todo.CreatedAt.Equal(stored.CreatedAt) || !todo.UpdatedAt.Equal(stored.UpdatedAt) {
				t.Fatalf("domain=%+v stored=%+v", todo, stored)
			}
		})
	}
}

func TestUpdateTodo(t *testing.T) {
	tests := []struct {
		name, before, after string
		missing, deleted    bool
	}{
		{name: "changed value", before: "before", after: "changed"},
		{name: "unchanged value", before: "same", after: "same"},
		{name: "zero value is persisted", before: "before"},
		{name: "missing row is a no-op", after: "changed", missing: true},
		{name: "deleted row stays deleted", before: "deleted", after: "changed", deleted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db := newTestTodoRepo(t)
			if !tt.missing {
				seedTodo(t, db, 1, tt.before, tt.deleted)
			}
			seedTodo(t, db, 2, "untouched", false)
			if err := repo.UpdateTodo(context.Background(), 1, &biz.UpdateTodoForm{Name: tt.after}); err != nil {
				t.Fatal(err)
			}
			var target model.Todo
			err := db.Unscoped().First(&target, 1).Error
			if tt.missing {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					t.Fatalf("missing row was created: %+v error=%v", target, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				want := tt.after
				if tt.deleted {
					want = tt.before
				}
				if target.Name != want || target.DeletedAt.Valid != tt.deleted {
					t.Fatalf("stored=%+v", target)
				}
			}
			other, err := repo.GetTodo(context.Background(), 2)
			if err != nil || other.Name != "untouched" {
				t.Fatalf("other row=%+v error=%v", other, err)
			}
		})
	}
}

func TestDeleteTodo(t *testing.T) {
	tests := []struct {
		name             string
		missing, deleted bool
	}{
		{name: "soft delete"}, {name: "missing", missing: true}, {name: "already deleted", deleted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db := newTestTodoRepo(t)
			if !tt.missing {
				seedTodo(t, db, 1, "test", tt.deleted)
			}
			seedTodo(t, db, 2, "untouched", false)
			err := repo.DeleteTodo(context.Background(), 1)
			if tt.missing || tt.deleted {
				if !errno.IsTodoNotFound(err) {
					t.Fatalf("delete error=%v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := repo.GetTodo(context.Background(), 1); !errno.IsTodoNotFound(err) {
					t.Fatalf("deleted row visible: %v", err)
				}
				var record model.Todo
				if err := db.Unscoped().First(&record, 1).Error; err != nil {
					t.Fatal(err)
				}
				if !record.DeletedAt.Valid || record.Name != "test" {
					t.Fatalf("soft-deleted record=%+v", record)
				}
			}
			if _, err := repo.GetTodo(context.Background(), 2); err != nil {
				t.Fatalf("unrelated row: %v", err)
			}
		})
	}
}

func TestListTodo(t *testing.T) {
	tests := []struct {
		name, filter string
		opts         []aip.ListOption
		ids          []int32
		total        int64
	}{
		{name: "default order excludes deleted", ids: []int32{1, 2, 3, 5}, total: 4},
		{name: "descending page", opts: []aip.ListOption{aip.ListOrderBy(ordering.OrderBy{Fields: []ordering.Field{{Path: "id", Desc: true}}}), aip.ListOffset(1), aip.ListLimit(2)}, ids: []int32{3, 2}, total: 4},
		{name: "empty page retains total", opts: []aip.ListOption{aip.ListOffset(10)}, total: 4},
		{name: "deterministic name ties", opts: []aip.ListOption{aip.ListOrderBy(ordering.OrderBy{Fields: []ordering.Field{{Path: "name"}}})}, ids: []int32{2, 1, 3, 5}, total: 4},
		{name: "created at descending", opts: []aip.ListOption{aip.ListOrderBy(ordering.OrderBy{Fields: []ordering.Field{{Path: "created_at", Desc: true}}})}, ids: []int32{5, 3, 2, 1}, total: 4},
		{name: "name filter", filter: `name = "beta"`, ids: []int32{1, 3}, total: 2},
		{name: "filtered page retains count", filter: `name = "beta"`, opts: []aip.ListOption{aip.ListOrderBy(ordering.OrderBy{Fields: []ordering.Field{{Path: "id", Desc: true}}}), aip.ListOffset(1), aip.ListLimit(1)}, ids: []int32{1}, total: 2},
		{name: "filtered empty page retains count", filter: `name = "beta"`, opts: []aip.ListOption{aip.ListOffset(10)}, total: 2},
		{name: "no matches", filter: `name = "absent"`},
		{name: "deleted row never matches", filter: `name = "deleted"`},
		{name: "compound filter", filter: `id > 1 AND name != "gamma"`, ids: []int32{2, 3}, total: 2},
		{name: "timestamp filter", filter: `updated_at >= timestamp("2026-01-03T00:00:00Z")`, ids: []int32{3, 5}, total: 2},
		{name: "SQL metacharacters are literal", filter: `name = "beta' OR 1=1 --"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db := newTestTodoRepo(t)
			for i, name := range []string{"beta", "alpha", "beta", "deleted", "gamma"} {
				seedTodo(t, db, int32(i+1), name, i == 3)
			}
			opts := append([]aip.ListOption{aip.ListFilter(parseTodoFilter(t, tt.filter))}, tt.opts...)
			todos, total, err := repo.ListTodo(context.Background(), opts...)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int32, 0, len(todos))
			for _, todo := range todos {
				ids = append(ids, todo.ID)
			}
			if total != tt.total || !slices.Equal(ids, tt.ids) {
				t.Fatalf("IDs=%v total=%d, want IDs=%v total=%d", ids, total, tt.ids, tt.total)
			}
		})
	}
	t.Run("default limit", func(t *testing.T) {
		repo, db := newTestTodoRepo(t)
		for i := int32(1); i <= 21; i++ {
			seedTodo(t, db, i, "test", false)
		}
		todos, total, err := repo.ListTodo(context.Background())
		if err != nil || total != 21 || len(todos) != 20 {
			t.Fatalf("count=%d total=%d error=%v", len(todos), total, err)
		}
	})
}

func TestListTodoTranslationErrors(t *testing.T) {
	tests := []struct {
		name   string
		option aip.ListOption
	}{
		{name: "malformed filter", option: aip.ListFilter(filtering.Filter{CheckedExpr: &expr.CheckedExpr{}})},
		{name: "unsafe order path", option: aip.ListOrderBy(ordering.OrderBy{Fields: []ordering.Field{{Path: "id; DROP TABLE todo"}}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newTestTodoRepo(t)
			todos, total, err := repo.ListTodo(context.Background(), tt.option)
			if !errno.IsInvalidArgument(err) || todos != nil || total != 0 {
				t.Fatalf("todos=%+v total=%d error=%v", todos, total, err)
			}
		})
	}
}

func TestTodoRepoStorageErrors(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, biz.TodoRepo) error
	}{
		{name: "create", call: func(ctx context.Context, r biz.TodoRepo) error {
			_, err := r.CreateTodo(ctx, &biz.Todo{Name: "test"})
			return err
		}},
		{name: "get", call: func(ctx context.Context, r biz.TodoRepo) error { _, err := r.GetTodo(ctx, 1); return err }},
		{name: "list", call: func(ctx context.Context, r biz.TodoRepo) error { _, _, err := r.ListTodo(ctx); return err }},
		{name: "update", call: func(ctx context.Context, r biz.TodoRepo) error {
			return r.UpdateTodo(ctx, 1, &biz.UpdateTodoForm{Name: "changed"})
		}},
		{name: "delete", call: func(ctx context.Context, r biz.TodoRepo) error { return r.DeleteTodo(ctx, 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db := newTestTodoRepo(t)
			seedTodo(t, db, 1, "before", false)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := tt.call(ctx, repo)
			if !errno.IsReasonUnspecified(err) || !errors.Is(err, context.Canceled) {
				t.Fatalf("storage error=%v", err)
			}
			todo, err := repo.GetTodo(context.Background(), 1)
			if err != nil || todo.Name != "before" {
				t.Fatalf("failed request mutated row: %+v error=%v", todo, err)
			}
		})
	}
}

func TestListTodoSQLOrdering(t *testing.T) {
	tests := []struct {
		name   string
		fields []ordering.Field
		want   string
	}{
		{name: "default ID", want: "`id`"},
		{name: "ID ascending", fields: []ordering.Field{{Path: "id"}}, want: "`id`"},
		{name: "ID descending", fields: []ordering.Field{{Path: "id", Desc: true}}, want: "`id` DESC"},
		{name: "name tie breaker", fields: []ordering.Field{{Path: "name", Desc: true}}, want: "`name` DESC,`id`"},
		{name: "explicit secondary ID", fields: []ordering.Field{{Path: "name"}, {Path: "id", Desc: true}}, want: "`name`,`id` DESC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, db := newTestTodoRepo(t)
			var selectSQL string
			if err := db.Callback().Query().After("gorm:query").Register("test:capture_todo_select", func(tx *gorm.DB) {
				sql := tx.Statement.SQL.String()
				if strings.HasPrefix(sql, "SELECT * FROM `todo`") {
					selectSQL = sql
				}
			}); err != nil {
				t.Fatal(err)
			}
			_, _, err := repo.ListTodo(context.Background(),
				aip.ListOrderBy(ordering.OrderBy{Fields: tt.fields}),
				aip.ListLimit(2), aip.ListOffset(1),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, order, ok := strings.Cut(selectSQL, " ORDER BY ")
			if !ok {
				t.Fatalf("missing order clause: %s", selectSQL)
			}
			order, _, _ = strings.Cut(order, " LIMIT ")
			if order != tt.want {
				t.Fatalf("ORDER BY %s, want %s; SQL: %s", order, tt.want, selectSQL)
			}
		})
	}
}
