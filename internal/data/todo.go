package data

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/flightzw/kratos-layout/internal/biz"
	"github.com/flightzw/kratos-layout/internal/data/model"
	"github.com/flightzw/kratos-layout/pkg/aip"
	aipgorm "github.com/flightzw/kratos-layout/pkg/aip/gorm"
)

type todoRepo struct {
	data *Data
}

// NewTodoRepo creates a repository using the shared database query client.
func NewTodoRepo(data *Data) biz.TodoRepo {
	return &todoRepo{data: data}
}

func (t *todoRepo) CreateTodo(ctx context.Context, todo *biz.Todo) (int32, error) {
	record := newTodo(todo)
	if err := t.data.query.Todo.WithContext(ctx).Create(record); err != nil {
		return 0, todoError("create todo", err)
	}
	return record.ID, nil
}

func (t *todoRepo) GetTodo(ctx context.Context, id int32) (*biz.Todo, error) {
	q := t.data.query.Todo
	record, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		return nil, todoError("get todo", err)
	}
	return toBiz(record), nil
}

func (t *todoRepo) ListTodo(ctx context.Context, opts ...aip.ListOption) (data []*biz.Todo, total int64, err error) {
	if total, err = aipgorm.FindByPage(t.data.db.WithContext(ctx), &data, opts...); err != nil {
		return nil, 0, todoError("list todo", err)
	}
	return data, total, nil
}

func (t *todoRepo) UpdateTodo(ctx context.Context, id int32, form *biz.UpdateTodoForm) error {
	q := t.data.query.Todo
	if _, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Update(q.Name, form.Name); err != nil {
		return todoError("update todo", err)
	}
	return nil
}

func (t *todoRepo) DeleteTodo(ctx context.Context, id int32) error {
	q := t.data.query.Todo
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	if err != nil {
		return todoError("delete todo", err)
	}
	if result.RowsAffected == 0 {
		return biz.ErrTodoNotFound
	}
	return nil
}

func newTodo(todo *biz.Todo) *model.Todo {
	return &model.Todo{
		ID:        todo.ID,
		Name:      todo.Name,
		CreatedAt: todo.CreatedAt,
		UpdatedAt: todo.UpdatedAt,
	}
}

func toBiz(todo *model.Todo) *biz.Todo {
	return &biz.Todo{
		ID:        todo.ID,
		Name:      todo.Name,
		CreatedAt: todo.CreatedAt,
		UpdatedAt: todo.UpdatedAt,
	}
}

func todoError(operation string, err error) error {
	if errors.Is(err, aipgorm.ErrInvalidQuery) {
		return biz.ErrTodoInvalidArgument.WithCause(fmt.Errorf("%s: %w", operation, err))
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return biz.ErrTodoNotFound
	}
	return biz.ErrTodoStorage.WithCause(fmt.Errorf("%s: %w", operation, err))
}
