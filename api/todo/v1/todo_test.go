package v1_test

import (
	"errors"
	"strings"
	"testing"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/proto"

	v1 "github.com/flightzw/kratos-layout/api/todo/v1"
)

func TestTodoRequestCEL(t *testing.T) {
	validator, err := protovalidate.New(protovalidate.WithMessages(
		&v1.CreateTodoRequest{}, &v1.GetTodoRequest{}, &v1.ListTodoRequest{},
		&v1.UpdateTodoRequest{}, &v1.DeleteTodoRequest{},
	), protovalidate.WithDisableLazy())
	if err != nil {
		t.Fatalf("compile CEL rules: %v", err)
	}
	tests := []struct {
		name          string
		request       proto.Message
		rule, message string
	}{
		{name: "create valid", request: &v1.CreateTodoRequest{Name: "test"}},
		{name: "create boundary", request: &v1.CreateTodoRequest{Name: strings.Repeat("中", 20)}},
		{name: "create surrounding whitespace", request: &v1.CreateTodoRequest{Name: " test "}},
		{name: "create missing name", request: &v1.CreateTodoRequest{}, rule: "name", message: "name 不得为空或仅包含空白字符"},
		{name: "create ASCII whitespace", request: &v1.CreateTodoRequest{Name: " \t\n\r\v\f"}, rule: "name", message: "name 不得为空或仅包含空白字符"},
		{name: "create Unicode whitespace", request: &v1.CreateTodoRequest{Name: "\u0085\u00a0\u1680\u2000\u200a\u2028\u2029\u202f\u205f\u3000"}, rule: "name", message: "name 不得为空或仅包含空白字符"},
		{name: "create too long", request: &v1.CreateTodoRequest{Name: strings.Repeat("中", 21)}, rule: "name", message: "name 长度不得超过 20 个字符"},
		{name: "get valid", request: &v1.GetTodoRequest{Id: 1}},
		{name: "get zero ID", request: &v1.GetTodoRequest{}, rule: "get_todo.id.positive", message: "id 必须大于 0"},
		{name: "get negative ID", request: &v1.GetTodoRequest{Id: -1}, rule: "get_todo.id.positive", message: "id 必须大于 0"},
		{name: "list defaults", request: &v1.ListTodoRequest{}},
		{name: "list explicit zero", request: &v1.ListTodoRequest{Page: 0, PageSize: 0}},
		{name: "list size boundary", request: &v1.ListTodoRequest{PageSize: 100}},
		{name: "list descending", request: &v1.ListTodoRequest{Sort: "name", Order: "desc"}},
		{name: "list ascending", request: &v1.ListTodoRequest{Sort: "id", Order: "asc"}},
		{name: "list created at", request: &v1.ListTodoRequest{Sort: "created_at"}},
		{name: "list updated at with spaces", request: &v1.ListTodoRequest{Sort: " updated_at "}},
		{name: "list maximum offset", request: &v1.ListTodoRequest{Page: 2147483647, PageSize: 1}},
		{name: "list negative page", request: &v1.ListTodoRequest{Page: -1}, rule: "list_todo.page.non_negative", message: "page 不得小于 0"},
		{name: "list negative size", request: &v1.ListTodoRequest{PageSize: -1}, rule: "list_todo.page_size.range", message: "page_size 必须在 0 到 100 之间"},
		{name: "list oversized page", request: &v1.ListTodoRequest{PageSize: 101}, rule: "list_todo.page_size.range", message: "page_size 必须在 0 到 100 之间"},
		{name: "list overflow", request: &v1.ListTodoRequest{Page: 2147483647, PageSize: 100}, rule: "list_todo.offset.range", message: "分页偏移量不得超过 2147483647"},
		{name: "list overflow with default size", request: &v1.ListTodoRequest{Page: 2147483647}, rule: "list_todo.offset.range", message: "分页偏移量不得超过 2147483647"},
		{name: "list invalid direction", request: &v1.ListTodoRequest{Order: "invalid"}, rule: "list_todo.order.allowed", message: "order 仅支持 asc 或 desc"},
		{name: "list unknown sort", request: &v1.ListTodoRequest{Sort: "missing"}, rule: "list_todo.sort.allowed", message: "sort 仅支持 id、name、created_at 或 updated_at"},
		{name: "list unsafe sort", request: &v1.ListTodoRequest{Sort: "id; DROP TABLE todo"}, rule: "list_todo.sort.allowed", message: "sort 仅支持 id、name、created_at 或 updated_at"},
		{name: "list multiple sorts", request: &v1.ListTodoRequest{Sort: "name,id"}, rule: "list_todo.sort.allowed", message: "sort 仅支持 id、name、created_at 或 updated_at"},
		{name: "update valid", request: &v1.UpdateTodoRequest{Id: 1, Form: &v1.UpdateTodoRequest_Form{Name: "changed"}}},
		{name: "update missing form", request: &v1.UpdateTodoRequest{Id: 1}, rule: "update_todo.form.present", message: "form 不得缺失"},
		{name: "update empty form", request: &v1.UpdateTodoRequest{Id: 1, Form: &v1.UpdateTodoRequest_Form{}}, rule: "name", message: "name 不得为空或仅包含空白字符"},
		{name: "update whitespace name", request: &v1.UpdateTodoRequest{Id: 1, Form: &v1.UpdateTodoRequest_Form{Name: "\u3000"}}, rule: "name", message: "name 不得为空或仅包含空白字符"},
		{name: "update long name", request: &v1.UpdateTodoRequest{Id: 1, Form: &v1.UpdateTodoRequest_Form{Name: strings.Repeat("中", 21)}}, rule: "name", message: "name 长度不得超过 20 个字符"},
		{name: "update zero ID", request: &v1.UpdateTodoRequest{Form: &v1.UpdateTodoRequest_Form{Name: "changed"}}, rule: "update_todo.id.positive", message: "id 必须大于 0"},
		{name: "update negative ID", request: &v1.UpdateTodoRequest{Id: -1, Form: &v1.UpdateTodoRequest_Form{Name: "changed"}}, rule: "update_todo.id.positive", message: "id 必须大于 0"},
		{name: "delete valid", request: &v1.DeleteTodoRequest{Id: 1}},
		{name: "delete zero ID", request: &v1.DeleteTodoRequest{}, rule: "delete_todo.id.positive", message: "id 必须大于 0"},
		{name: "delete negative ID", request: &v1.DeleteTodoRequest{Id: -1}, rule: "delete_todo.id.positive", message: "id 必须大于 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.request)
			if tt.rule == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var validationError *protovalidate.ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("expected violation, got %v", err)
			}
			violations := validationError.ToProto().GetViolations()
			if len(violations) != 1 || violations[0].GetRuleId() != tt.rule || violations[0].GetMessage() != tt.message {
				t.Fatalf("violations: %+v", violations)
			}
		})
	}
}
