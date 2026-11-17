package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flightzw/kratos-layout/api/commons/errno"
	"github.com/flightzw/kratos-layout/internal/biz"
	"github.com/flightzw/kratos-layout/internal/data/query"
)

func TestTodoTransaction(t *testing.T) {
	rollback := errors.New("rollback requested")
	tests := []struct {
		name   string
		result error
	}{
		{name: "commit"},
		{name: "rollback", result: rollback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, db := newTestTodoRepo(t)
			d := &Data{db: db, query: query.Use(db)}
			writer, reader := NewTodoRepo(d), NewTodoRepo(d)
			seedTodo(t, db, 1, "original", false)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var createdID int32
			err := d.Transaction(ctx, func(txCtx context.Context) error {
				var err error
				createdID, err = writer.CreateTodo(txCtx, &biz.Todo{Name: "created"})
				if err != nil {
					return err
				}
				created, err := reader.GetTodo(txCtx, createdID)
				if err != nil {
					return err
				}
				if created.Name != "created" {
					t.Errorf("created name=%q", created.Name)
				}
				if err := writer.UpdateTodo(txCtx, createdID, &biz.UpdateTodoForm{Name: "updated"}); err != nil {
					return err
				}
				updated, err := reader.GetTodo(txCtx, createdID)
				if err != nil {
					return err
				}
				if updated.Name != "updated" {
					t.Errorf("updated name=%q", updated.Name)
				}
				if err := writer.DeleteTodo(txCtx, 1); err != nil {
					return err
				}
				if _, err := reader.GetTodo(txCtx, 1); !errno.IsTodoNotFound(err) {
					t.Errorf("deleted row visible inside transaction: %v", err)
				}
				todos, total, err := reader.ListTodo(txCtx)
				if err != nil {
					return err
				}
				if total != 1 || len(todos) != 1 || todos[0].ID != createdID || todos[0].Name != "updated" {
					t.Errorf("transaction list=%+v total=%d", todos, total)
				}
				return tt.result
			})
			if !errors.Is(err, tt.result) {
				t.Fatalf("transaction error=%v, want %v", err, tt.result)
			}
			created, err := reader.GetTodo(ctx, createdID)
			if tt.result == nil {
				if err != nil || created.Name != "updated" {
					t.Fatalf("committed row=%+v error=%v", created, err)
				}
				if _, err := reader.GetTodo(ctx, 1); !errno.IsTodoNotFound(err) {
					t.Fatalf("delete not committed: %v", err)
				}
			} else {
				if !errno.IsTodoNotFound(err) {
					t.Fatalf("created row survived rollback: %+v error=%v", created, err)
				}
				original, err := reader.GetTodo(ctx, 1)
				if err != nil || original.Name != "original" {
					t.Fatalf("original row=%+v error=%v", original, err)
				}
			}
			todos, total, err := reader.ListTodo(ctx)
			if err != nil || total != 1 || len(todos) != 1 {
				t.Fatalf("list after transaction=%+v total=%d error=%v", todos, total, err)
			}
		})
	}
}

func TestTransactionPanicRollsBack(t *testing.T) {
	_, db := newTestTodoRepo(t)
	d := &Data{db: db, query: query.Use(db)}
	repo := NewTodoRepo(d)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failure := errors.New("callback panic")
	var id int32
	func() {
		defer func() {
			if got, ok := recover().(error); !ok || !errors.Is(got, failure) {
				t.Errorf("panic=%v, want callback panic", got)
			}
		}()
		err := d.Transaction(ctx, func(txCtx context.Context) error {
			var err error
			id, err = repo.CreateTodo(txCtx, &biz.Todo{Name: "rollback"})
			if err != nil {
				return err
			}
			panic(failure)
		})
		t.Errorf("transaction did not propagate panic: %v", err)
	}()
	if _, err := repo.GetTodo(ctx, id); !errno.IsTodoNotFound(err) {
		t.Fatalf("panic did not roll back: %v", err)
	}
}

func TestNestedTransaction(t *testing.T) {
	_, db := newTestTodoRepo(t)
	d := &Data{db: db, query: query.Use(db)}
	repo := NewTodoRepo(d)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rollback := errors.New("rollback inner transaction")
	var innerID int32
	err := d.Transaction(ctx, func(outerCtx context.Context) error {
		if _, err := repo.CreateTodo(outerCtx, &biz.Todo{Name: "before"}); err != nil {
			return err
		}
		err := d.Transaction(outerCtx, func(innerCtx context.Context) error {
			var err error
			innerID, err = repo.CreateTodo(innerCtx, &biz.Todo{Name: "inner"})
			if err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			return err
		}
		if _, err := repo.GetTodo(outerCtx, innerID); !errno.IsTodoNotFound(err) {
			t.Errorf("inner row survived rollback: %v", err)
		}
		_, err = repo.CreateTodo(outerCtx, &biz.Todo{Name: "after"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	todos, total, err := repo.ListTodo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(todos))
	for _, todo := range todos {
		names[todo.Name] = true
	}
	if total != 2 || len(todos) != 2 || !names["before"] || !names["after"] {
		t.Fatalf("rows after nested transaction=%+v total=%d", todos, total)
	}
}

func TestTransactionCancelledContext(t *testing.T) {
	_, db := newTestTodoRepo(t)
	d := &Data{db: db, query: query.Use(db)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := d.Transaction(ctx, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("error=%v callback called=%v", err, called)
	}
}

func TestNestedTransactionOuterRollback(t *testing.T) {
	_, db := newTestTodoRepo(t)
	d := &Data{db: db, query: query.Use(db)}
	repo := NewTodoRepo(d)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rollback := errors.New("rollback outer transaction")
	err := d.Transaction(ctx, func(outerCtx context.Context) error {
		if err := d.Transaction(outerCtx, func(innerCtx context.Context) error {
			_, err := repo.CreateTodo(innerCtx, &biz.Todo{Name: "inner"})
			return err
		}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error=%v", err)
	}
	todos, total, err := repo.ListTodo(ctx)
	if err != nil || total != 0 || len(todos) != 0 {
		t.Fatalf("inner write survived outer rollback: %+v total=%d error=%v", todos, total, err)
	}
}
