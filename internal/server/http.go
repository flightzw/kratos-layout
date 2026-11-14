package server

import (
	"log/slog"
	"strings"

	"github.com/go-kratos/kratos/v3/middleware/logging"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/transport/http"

	v1 "github.com/flightzw/kratos-layout/api/todo/v1"
	"github.com/flightzw/kratos-layout/docs"
	"github.com/flightzw/kratos-layout/internal/conf"
	"github.com/flightzw/kratos-layout/internal/service"
	"github.com/flightzw/kratos-layout/pkg/apidoc"
	"github.com/flightzw/kratos-layout/pkg/middleware/validate"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, todo *service.TodoService, logger *slog.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			logging.Server(logger),
			validate.Validator(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	v1.RegisterTodoServiceHTTPServer(srv, todo)
	apidoc.RegisterScalarDocHTTPServer(srv, apidoc.ScalarConfig{
		Port:    parsePort(c.Http.Addr),
		Docs:    docs.APISpec,
		Path:    "/openapi/reference",
		SpecURL: "http://localhost:" + parsePort(c.Http.Addr) + "/openapi/spec.json",
	}, apidoc.UseDefaultScalarOptions("Todo API Reference")...)
	return srv
}

func parsePort(addr string) string {
	return addr[strings.LastIndex(addr, ":")+1:]
}
