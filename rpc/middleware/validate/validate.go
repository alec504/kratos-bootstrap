// Package validate provides request validation middleware compatible with both
// Protovalidate and the legacy protoc-gen-validate generated interface.
package validate

import (
	"context"
	stderrors "errors"

	"buf.build/go/protovalidate"
	kratosErrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"google.golang.org/protobuf/proto"
)

const (
	validatorReason         = "VALIDATOR"
	validatorInternalReason = "VALIDATOR_INTERNAL"
)

type legacyValidator interface {
	Validate() error
}

type protoValidateFunc func(proto.Message, ...protovalidate.ValidationOption) error

// ProtoValidate validates protobuf requests with Protovalidate and then falls
// back to the legacy protoc-gen-validate interface when it is implemented.
// This allows callers to migrate contracts incrementally without changing the
// enable_validate configuration.
func ProtoValidate() middleware.Middleware {
	return protoValidate(protovalidate.Validate)
}

func protoValidate(validate protoValidateFunc) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (reply any, err error) {
			if msg, ok := req.(proto.Message); ok {
				if err := validate(msg); err != nil {
					var validationErr *protovalidate.ValidationError
					if stderrors.As(err, &validationErr) {
						return nil, kratosErrors.BadRequest(validatorReason, err.Error()).WithCause(err)
					}
					return nil, kratosErrors.InternalServer(
						validatorInternalReason,
						"validation schema error",
					).WithCause(err)
				}
			}

			if validator, ok := req.(legacyValidator); ok {
				if err := validator.Validate(); err != nil {
					return nil, kratosErrors.BadRequest(validatorReason, err.Error()).WithCause(err)
				}
			}

			return handler(ctx, req)
		}
	}
}
