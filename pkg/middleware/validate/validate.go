package validate

import (
	"context"

	"buf.build/go/protovalidate"
	"github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/middleware"
	"google.golang.org/protobuf/proto"
)

// validateProto applies request CEL rules consistently to HTTP and gRPC.
func Validator() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if msg, ok := req.(proto.Message); ok {
				if err := protovalidate.Validate(msg); err != nil {
					return nil, errors.BadRequest("VALIDATOR", err.Error())
				}
			}
			return next(ctx, req)
		}
	}
}
