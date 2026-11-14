package biz

import (
	"context"

	"github.com/flightzw/kratos-layout/api/commons/errno"
	"github.com/flightzw/kratos-layout/internal/data/model"
	"github.com/flightzw/kratos-layout/pkg/aip"
)

var (
	ErrTodoNotFound        = errno.ErrorTodoNotFound("todo 不存在")
	ErrTodoInvalidArgument = errno.ErrorInvalidArgument("todo 参数无效")
	ErrTodoStorage         = errno.ErrorReasonUnspecified("todo 存储操作失败")
)

// Todo is the domain model, independent of transport and storage.
type Todo = model.Todo

type UpdateTodoForm struct {
	Name string
}

// TodoRepo is the storage boundary for todos.
type TodoRepo interface {
	CreateTodo(ctx context.Context, data *Todo) (int32, error)
	ListTodo(ctx context.Context, opts ...aip.ListOption) ([]*Todo, int64, error)
	GetTodo(ctx context.Context, id int32) (*Todo, error)
	UpdateTodo(ctx context.Context, id int32, form *UpdateTodoForm) error
	DeleteTodo(ctx context.Context, id int32) error
}

// TodoUsecase is a Todo usecase.
type TodoUsecase struct {
	repo TodoRepo
}

// NewTodoUsecase new a Todo usecase.
func NewTodoUsecase(repo TodoRepo) *TodoUsecase {
	return &TodoUsecase{repo: repo}
}

func (uc *TodoUsecase) CreateTodo(ctx context.Context, todo *Todo) (int32, error) {
	return uc.repo.CreateTodo(ctx, todo)
}

func (uc *TodoUsecase) ListTodo(ctx context.Context, opts ...aip.ListOption) ([]*Todo, int64, error) {
	return uc.repo.ListTodo(ctx, opts...)
}

func (uc *TodoUsecase) GetTodo(ctx context.Context, id int32) (*Todo, error) {
	return uc.repo.GetTodo(ctx, id)
}

func (uc *TodoUsecase) UpdateTodo(ctx context.Context, id int32, form *UpdateTodoForm) error {
	return uc.repo.UpdateTodo(ctx, id, form)
}

func (uc *TodoUsecase) DeleteTodo(ctx context.Context, id int32) error {
	return uc.repo.DeleteTodo(ctx, id)
}
