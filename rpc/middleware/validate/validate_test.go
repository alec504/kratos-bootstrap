package validate

import (
	"context"
	"errors"
	"testing"

	"buf.build/go/protovalidate"
	kratosErrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type legacyRequest struct {
	err   error
	calls int
}

func (r *legacyRequest) Validate() error {
	r.calls++
	return r.err
}

type mixedRequest struct {
	*emptypb.Empty
	legacyRequest
}

func TestProtoValidateLegacyFailure(t *testing.T) {
	req := &legacyRequest{err: errors.New("legacy validation failed")}
	handlerCalled := false

	handler := protoValidate(func(proto.Message, ...protovalidate.ValidationOption) error {
		t.Fatal("protovalidate must not run for a non-protobuf request")
		return nil
	})(func(context.Context, any) (any, error) {
		handlerCalled = true
		return nil, nil
	})

	_, err := handler(context.Background(), req)
	if !kratosErrors.IsBadRequest(err) {
		t.Fatalf("expected bad request, got %v", err)
	}
	if req.calls != 1 {
		t.Fatalf("expected one legacy validation call, got %d", req.calls)
	}
	if handlerCalled {
		t.Fatal("handler must not run after validation failure")
	}
}

func TestProtoValidateModernFailure(t *testing.T) {
	req := &emptypb.Empty{}
	handlerCalled := false

	handler := protoValidate(func(proto.Message, ...protovalidate.ValidationOption) error {
		return &protovalidate.ValidationError{}
	})(func(context.Context, any) (any, error) {
		handlerCalled = true
		return nil, nil
	})

	_, err := handler(context.Background(), req)
	if !kratosErrors.IsBadRequest(err) {
		t.Fatalf("expected bad request, got %v", err)
	}
	if handlerCalled {
		t.Fatal("handler must not run after validation failure")
	}
}

func TestProtoValidateInternalFailure(t *testing.T) {
	req := &emptypb.Empty{}

	handler := protoValidate(func(proto.Message, ...protovalidate.ValidationOption) error {
		return &protovalidate.CompilationError{}
	})(func(context.Context, any) (any, error) {
		t.Fatal("handler must not run after validation failure")
		return nil, nil
	})

	_, err := handler(context.Background(), req)
	if !kratosErrors.IsInternalServer(err) {
		t.Fatalf("expected internal server error, got %v", err)
	}
	if kratosErrors.Reason(err) != validatorInternalReason {
		t.Fatalf("expected reason %s, got %v", validatorInternalReason, err)
	}
}

func TestProtoValidateMixedRunsLegacyAfterProtovalidate(t *testing.T) {
	req := &mixedRequest{
		Empty:         &emptypb.Empty{},
		legacyRequest: legacyRequest{err: errors.New("legacy validation failed")},
	}
	protoCalls := 0

	handler := protoValidate(func(proto.Message, ...protovalidate.ValidationOption) error {
		protoCalls++
		return nil
	})(func(context.Context, any) (any, error) {
		t.Fatal("handler must not run after validation failure")
		return nil, nil
	})

	_, err := handler(context.Background(), req)
	if !kratosErrors.IsBadRequest(err) {
		t.Fatalf("expected bad request, got %v", err)
	}
	if protoCalls != 1 {
		t.Fatalf("expected one protovalidate call, got %d", protoCalls)
	}
	if req.calls != 1 {
		t.Fatalf("expected one legacy validation call, got %d", req.calls)
	}
}

func TestProtoValidateStopsBeforeLegacyOnModernFailure(t *testing.T) {
	req := &mixedRequest{Empty: &emptypb.Empty{}}

	handler := protoValidate(func(proto.Message, ...protovalidate.ValidationOption) error {
		return &protovalidate.ValidationError{}
	})(func(context.Context, any) (any, error) {
		t.Fatal("handler must not run after validation failure")
		return nil, nil
	})

	_, err := handler(context.Background(), req)
	if !kratosErrors.IsBadRequest(err) {
		t.Fatalf("expected bad request, got %v", err)
	}
	if req.calls != 0 {
		t.Fatalf("legacy validation must not run, got %d calls", req.calls)
	}
}

func TestProtoValidatePassThrough(t *testing.T) {
	req := &mixedRequest{Empty: &emptypb.Empty{}}
	want := &emptypb.Empty{}
	var handler middleware.Handler = func(context.Context, any) (any, error) {
		return want, nil
	}

	got, err := protoValidate(func(proto.Message, ...protovalidate.ValidationOption) error {
		return nil
	})(handler)(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected reply: got %v want %v", got, want)
	}
	if req.calls != 1 {
		t.Fatalf("expected one legacy validation call, got %d", req.calls)
	}
}
