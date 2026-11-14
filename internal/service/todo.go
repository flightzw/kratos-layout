package service

import (
	"context"

	"github.com/gogf/gf/v2/util/gconv"
	"go.einride.tech/aip/filtering"

	v1 "github.com/flightzw/kratos-layout/api/todo/v1"
	"github.com/flightzw/kratos-layout/internal/biz"
	"github.com/flightzw/kratos-layout/pkg/aip"
)

// TodoService adapts the Todo API to the domain usecase.
type TodoService struct {
	v1.UnimplementedTodoServiceServer

	uc   *biz.TodoUsecase
	decl *filtering.Declarations
}

// NewTodoService creates a service with the required usecase supplied by Wire.
func NewTodoService(uc *biz.TodoUsecase) (*TodoService, error) {
	declarations, err := filtering.NewDeclarations(
		filtering.DeclareStandardFunctions(),
		filtering.DeclareIdent("id", filtering.TypeInt),
		filtering.DeclareIdent("name", filtering.TypeString),
		filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("updated_at", filtering.TypeTimestamp),
	)
	if err != nil {
		return nil, err
	}
	return &TodoService{uc: uc, decl: declarations}, nil
}

func (s *TodoService) CreateTodo(ctx context.Context, req *v1.CreateTodoRequest) (*v1.CreateTodoReply, error) {
	if _, err := s.uc.CreateTodo(ctx, &biz.Todo{
		Name: req.Name,
	}); err != nil {
		return nil, err
	}
	return &v1.CreateTodoReply{}, nil
}

func (s *TodoService) GetTodo(ctx context.Context, req *v1.GetTodoRequest) (*v1.GetTodoReply, error) {
	todo, err := s.uc.GetTodo(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	data := &v1.Todo{}
	if err := gconv.Struct(todo, data); err != nil {
		return nil, err
	}
	return &v1.GetTodoReply{Data: data}, nil
}

func (s *TodoService) ListTodo(ctx context.Context, req *v1.ListTodoRequest) (*v1.ListTodoReply, error) {
	opts, err := aip.ParsePaginationOptions(req)
	if err != nil {
		return nil, err
	}
	filter, err := aip.ParseFilter(req, s.decl)
	if err != nil {
		return nil, err
	}
	todos, total, err := s.uc.ListTodo(ctx, append(opts, aip.ListFilter(filter))...)
	if err != nil {
		return nil, err
	}
	data := make([]*v1.Todo, 0, len(todos))
	if err := gconv.Structs(todos, &data); err != nil {
		return nil, err
	}
	return &v1.ListTodoReply{Data: data, Total: total}, nil
}

func (s *TodoService) UpdateTodo(ctx context.Context, req *v1.UpdateTodoRequest) (*v1.UpdateTodoReply, error) {
	form := req.Form
	if err := s.uc.UpdateTodo(ctx, req.Id, &biz.UpdateTodoForm{Name: form.Name}); err != nil {
		return nil, err
	}
	return &v1.UpdateTodoReply{}, nil
}

func (s *TodoService) DeleteTodo(ctx context.Context, req *v1.DeleteTodoRequest) (*v1.DeleteTodoReply, error) {
	if err := s.uc.DeleteTodo(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &v1.DeleteTodoReply{}, nil
}
