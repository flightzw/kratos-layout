package service

import (
	"context"
	"errors"
	"testing"
	"time"

	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/gogf/gf/v2/os/gtime"
	"go.einride.tech/aip/filtering"
	"google.golang.org/protobuf/proto"

	v1 "github.com/flightzw/kratos-layout/api/todo/v1"
	"github.com/flightzw/kratos-layout/internal/biz"
	"github.com/flightzw/kratos-layout/pkg/aip"
)

// The concrete usecase forwards to this test-only repository, as in production.
type todoRepoStub struct {
	todo    *biz.Todo
	todos   []*biz.Todo
	total   int64
	err     error
	calls   int
	ctx     context.Context
	gotTodo *biz.Todo
	gotForm *biz.UpdateTodoForm
	gotID   int32
	options aip.ListOptions
}

var _ biz.TodoRepo = (*todoRepoStub)(nil)

func (r *todoRepoStub) record(ctx context.Context) {
	r.calls++
	r.ctx = ctx
}
func (r *todoRepoStub) CreateTodo(ctx context.Context, todo *biz.Todo) (int32, error) {
	r.record(ctx)
	r.gotTodo = todo
	return 9, r.err
}
func (r *todoRepoStub) GetTodo(ctx context.Context, id int32) (*biz.Todo, error) {
	r.record(ctx)
	r.gotID = id
	return r.todo, r.err
}
func (r *todoRepoStub) ListTodo(ctx context.Context, opts ...aip.ListOption) ([]*biz.Todo, int64, error) {
	r.record(ctx)
	for _, opt := range opts {
		opt(&r.options)
	}
	return r.todos, r.total, r.err
}
func (r *todoRepoStub) UpdateTodo(ctx context.Context, id int32, form *biz.UpdateTodoForm) error {
	r.record(ctx)
	r.gotID = id
	r.gotForm = form
	return r.err
}
func (r *todoRepoStub) DeleteTodo(ctx context.Context, id int32) error {
	r.record(ctx)
	r.gotID = id
	return r.err
}

func newTestTodoService(t *testing.T, repo *todoRepoStub) *TodoService {
	t.Helper()
	svc, err := NewTodoService(biz.NewTodoUsecase(repo))
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}
	return svc
}

func assertTodoReply(t *testing.T, got *v1.Todo, want *biz.Todo) {
	t.Helper()
	if got == nil || got.Id != want.ID || got.Name != want.Name || got.CreatedAt != want.CreatedAt.String() || got.UpdatedAt != want.UpdatedAt.String() {
		t.Fatalf("DTO=%+v, want domain fields from %+v", got, want)
	}
}

func TestTodoServiceCRUD(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Run("create maps name", func(t *testing.T) {
		var repo todoRepoStub
		reply, err := newTestTodoService(t, &repo).CreateTodo(ctx, &v1.CreateTodoRequest{Name: "test"})
		if err != nil || reply == nil {
			t.Fatalf("reply=%+v error=%v", reply, err)
		}
		if repo.gotTodo == nil || repo.gotTodo.Name != "test" || repo.gotTodo.ID != 0 || !repo.gotTodo.CreatedAt.IsZero() || !repo.gotTodo.UpdatedAt.IsZero() {
			t.Fatalf("domain todo=%+v", repo.gotTodo)
		}
		if repo.calls != 1 || repo.ctx != ctx {
			t.Fatal("expected one call with original context")
		}
	})
	t.Run("get maps resource and timestamps", func(t *testing.T) {
		created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		repo := &todoRepoStub{todo: &biz.Todo{ID: 9, Name: "test", CreatedAt: gtime.New(created), UpdatedAt: gtime.New(created.Add(time.Hour))}}
		reply, err := newTestTodoService(t, repo).GetTodo(ctx, &v1.GetTodoRequest{Id: 9})
		if err != nil {
			t.Fatal(err)
		}
		assertTodoReply(t, reply.Data, repo.todo)
		if repo.gotID != 9 || repo.calls != 1 || repo.ctx != ctx {
			t.Fatalf("id=%d calls=%d context=%v", repo.gotID, repo.calls, repo.ctx)
		}
	})
	t.Run("update passes ID separately from form", func(t *testing.T) {
		var repo todoRepoStub
		reply, err := newTestTodoService(t, &repo).UpdateTodo(ctx, &v1.UpdateTodoRequest{Id: 9, Form: &v1.UpdateTodoRequest_Form{Name: "changed"}})
		if err != nil || reply == nil {
			t.Fatalf("reply=%+v error=%v", reply, err)
		}
		if repo.gotID != 9 || repo.gotForm == nil || repo.gotForm.Name != "changed" || repo.calls != 1 || repo.ctx != ctx {
			t.Fatalf("id=%d form=%+v calls=%d", repo.gotID, repo.gotForm, repo.calls)
		}
	})
	t.Run("delete passes ID", func(t *testing.T) {
		var repo todoRepoStub
		reply, err := newTestTodoService(t, &repo).DeleteTodo(ctx, &v1.DeleteTodoRequest{Id: 9})
		if err != nil || reply == nil {
			t.Fatalf("reply=%+v error=%v", reply, err)
		}
		if repo.gotID != 9 || repo.calls != 1 || repo.ctx != ctx {
			t.Fatalf("id=%d calls=%d", repo.gotID, repo.calls)
		}
	})
}

func TestListTodo(t *testing.T) {
	tests := []struct {
		name          string
		req           *v1.ListTodoRequest
		offset, limit int
		sort          string
		desc          bool
	}{
		{name: "defaults", req: &v1.ListTodoRequest{}, limit: 20, sort: "id"},
		{name: "explicit page and sort", req: &v1.ListTodoRequest{Page: 3, PageSize: 5, Sort: "name", Order: "desc"}, offset: 10, limit: 5, sort: "name", desc: true},
		{name: "maximum size", req: &v1.ListTodoRequest{PageSize: 100}, limit: 100, sort: "id"},
		{name: "maximum page", req: &v1.ListTodoRequest{Page: 2147483647, PageSize: 1}, offset: 2147483646, limit: 1, sort: "id"},
		{name: "created at", req: &v1.ListTodoRequest{Sort: "created_at"}, limit: 20, sort: "created_at"},
		{name: "updated at with spaces", req: &v1.ListTodoRequest{Sort: " updated_at "}, limit: 20, sort: "updated_at"},
		{name: "name filter", req: &v1.ListTodoRequest{Filter: `name = "test"`}, limit: 20, sort: "id"},
		{name: "ID filter", req: &v1.ListTodoRequest{Filter: "id >= 9"}, limit: 20, sort: "id"},
		{name: "created timestamp filter", req: &v1.ListTodoRequest{Filter: `created_at >= timestamp("2026-01-01T00:00:00Z")`}, limit: 20, sort: "id"},
		{name: "updated timestamp filter", req: &v1.ListTodoRequest{Filter: `updated_at < timestamp("2027-01-01T00:00:00Z")`}, limit: 20, sort: "id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			todo := &biz.Todo{ID: 9, Name: "test", CreatedAt: gtime.New(created), UpdatedAt: gtime.New(created.Add(time.Hour))}
			repo := &todoRepoStub{todos: []*biz.Todo{todo}, total: 42}
			svc := newTestTodoService(t, repo)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reply, err := svc.ListTodo(ctx, tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if reply.Total != 42 || len(reply.Data) != 1 {
				t.Fatalf("reply=%+v", reply)
			}
			assertTodoReply(t, reply.Data[0], todo)
			opts := repo.options
			if opts.Offset != tt.offset || opts.Limit != tt.limit || len(opts.OrderBy.Fields) != 1 {
				t.Fatalf("options=%+v", opts)
			}
			if order := opts.OrderBy.Fields[0]; order.Path != tt.sort || order.Desc != tt.desc {
				t.Fatalf("order=%+v", order)
			}
			expected, err := filtering.ParseFilterString(tt.req.Filter, svc.decl)
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(opts.Filter.CheckedExpr, expected.CheckedExpr) {
				t.Fatalf("filter was not forwarded: %+v", opts.Filter.CheckedExpr)
			}
			if repo.calls != 1 || repo.ctx != ctx {
				t.Fatal("expected one call with original context")
			}
		})
	}
}

func TestListTodoEmpty(t *testing.T) {
	reply, err := newTestTodoService(t, &todoRepoStub{}).ListTodo(context.Background(), &v1.ListTodoRequest{})
	if err != nil || reply == nil {
		t.Fatalf("reply=%+v error=%v", reply, err)
	}
	if reply.Total != 0 || len(reply.Data) != 0 {
		t.Fatalf("reply=%+v", reply)
	}
}

// Field constraints are covered by proto CEL tests; these are parsing failures.
func TestListTodoParseErrors(t *testing.T) {
	tests := []struct {
		name   string
		req    *v1.ListTodoRequest
		reason string
	}{
		{name: "invalid sort syntax", req: &v1.ListTodoRequest{Sort: "id; DROP TABLE todo"}, reason: "SORT_RULE_INVALID"},
		{name: "invalid direction syntax", req: &v1.ListTodoRequest{Order: "invalid"}, reason: "SORT_RULE_INVALID"},
		{name: "invalid filter syntax", req: &v1.ListTodoRequest{Filter: "name ="}, reason: "FILTER_RULE_INVALID"},
		{name: "undeclared filter field", req: &v1.ListTodoRequest{Filter: `missing = "test"`}, reason: "FILTER_RULE_INVALID"},
		{name: "filter type mismatch", req: &v1.ListTodoRequest{Filter: `id = "text"`}, reason: "FILTER_RULE_INVALID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var repo todoRepoStub
			reply, err := newTestTodoService(t, &repo).ListTodo(context.Background(), tt.req)
			if reply != nil || kerrors.Code(err) != 400 || kerrors.Reason(err) != tt.reason {
				t.Fatalf("reply=%+v error=%v", reply, err)
			}
			if repo.calls != 0 {
				t.Fatal("parse failure reached repo")
			}
		})
	}
}

func TestTodoServicePropagatesErrors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(*testing.T, *TodoService) error
	}{
		{name: "create", call: func(t *testing.T, s *TodoService) error {
			reply, err := s.CreateTodo(ctx, &v1.CreateTodoRequest{Name: "test"})
			if reply != nil {
				t.Errorf("unexpected reply: %+v", reply)
			}
			return err
		}},
		{name: "get", call: func(t *testing.T, s *TodoService) error {
			reply, err := s.GetTodo(ctx, &v1.GetTodoRequest{Id: 9})
			if reply != nil {
				t.Errorf("unexpected reply: %+v", reply)
			}
			return err
		}},
		{name: "list", call: func(t *testing.T, s *TodoService) error {
			reply, err := s.ListTodo(ctx, &v1.ListTodoRequest{})
			if reply != nil {
				t.Errorf("unexpected reply: %+v", reply)
			}
			return err
		}},
		{name: "update", call: func(t *testing.T, s *TodoService) error {
			reply, err := s.UpdateTodo(ctx, &v1.UpdateTodoRequest{Id: 9, Form: &v1.UpdateTodoRequest_Form{Name: "test"}})
			if reply != nil {
				t.Errorf("unexpected reply: %+v", reply)
			}
			return err
		}},
		{name: "delete", call: func(t *testing.T, s *TodoService) error {
			reply, err := s.DeleteTodo(ctx, &v1.DeleteTodoRequest{Id: 9})
			if reply != nil {
				t.Errorf("unexpected reply: %+v", reply)
			}
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := biz.ErrTodoStorage.WithCause(context.DeadlineExceeded)
			repo := &todoRepoStub{err: failure}
			err := tt.call(t, newTestTodoService(t, repo))
			if !errors.Is(err, failure) || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error=%v, want original failure", err)
			}
			if repo.calls != 1 || repo.ctx != ctx {
				t.Fatal("expected one call with original context")
			}
		})
	}
}

func TestGetTodoEmptyTimes(t *testing.T) {
	tests := []struct {
		name  string
		value *gtime.Time
	}{
		{name: "nil"},
		{name: "zero", value: gtime.New(time.Time{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &todoRepoStub{todo: &biz.Todo{ID: 1, Name: "test", CreatedAt: tt.value, UpdatedAt: tt.value}}
			reply, err := newTestTodoService(t, repo).GetTodo(context.Background(), &v1.GetTodoRequest{Id: 1})
			if err != nil {
				t.Fatal(err)
			}
			if reply.Data.CreatedAt != "" || reply.Data.UpdatedAt != "" {
				t.Fatalf("empty times: %+v", reply.Data)
			}
		})
	}
}
