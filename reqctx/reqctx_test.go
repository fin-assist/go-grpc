package reqctx_test

import (
	"context"
	"testing"

	"github.com/fin-assist/go-grpc/reqctx"
)

func TestUnitContextValuesAreIsolated(t *testing.T) {
	base := t.Context()
	ctx := reqctx.WithCaller(reqctx.WithUserID(reqctx.WithRequestID(base, "r1"), "u1"), "svc")
	ctx = reqctx.WithReadPrimary(reqctx.WithClientVersion(reqctx.WithClientIP(ctx, "127.0.0.1"), "v1"))
	if reqctx.RequestID(ctx) != "r1" || reqctx.Caller(ctx) != "svc" || reqctx.ClientIP(ctx) != "127.0.0.1" || reqctx.ClientVersion(ctx) != "v1" || !reqctx.ReadPrimary(ctx) {
		t.Fatal("lost scoped values")
	}
	if u, ok := reqctx.UserID(ctx); u != "u1" || !ok {
		t.Fatal("lost user")
	}
	if reqctx.RequestID(base) != "" || reqctx.Caller(base) != "" || reqctx.ReadPrimary(base) {
		t.Fatal("mutated parent")
	}
	if _, ok := reqctx.UserID(base); ok {
		t.Fatal("parent acquired identity")
	}
}
func TestAPIRequestContextComposition(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	derived := reqctx.WithReadPrimary(reqctx.WithClientIP(reqctx.WithClientVersion(reqctx.WithCaller(reqctx.WithUserID(reqctx.WithRequestID(ctx, "trace"), "subject"), "peer"), "android"), "192.0.2.1"))
	if reqctx.RequestID(derived) != "trace" || reqctx.Caller(derived) != "peer" || reqctx.ClientIP(derived) != "192.0.2.1" || reqctx.ClientVersion(derived) != "android" || !reqctx.ReadPrimary(derived) {
		t.Fatal("consumer context lost values")
	}
	if u, ok := reqctx.UserID(derived); u != "subject" || !ok {
		t.Fatal("consumer context lost subject")
	}
	cancel()
	if derived.Err() != context.Canceled {
		t.Fatal("context cancellation disconnected")
	}
}
