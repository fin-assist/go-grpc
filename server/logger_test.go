package server

import (
	"context"
	"io"
	"log/slog"

	"github.com/fin-assist/go-grpc/reqctx"
)

// The consumer owns its logger; this test handler proves request values remain available.
func testLogger(w io.Writer) *slog.Logger {
	return slog.New(testContextHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})})
}

type testContextHandler struct{ slog.Handler }

func (h testContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := reqctx.RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if c := reqctx.Caller(ctx); c != "" {
		r.AddAttrs(slog.String("caller", c))
	}
	if u, ok := reqctx.UserID(ctx); ok {
		r.AddAttrs(slog.String("user_id", u))
	}
	return h.Handler.Handle(ctx, r)
}
func (h testContextHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return testContextHandler{h.Handler.WithAttrs(a)}
}
func (h testContextHandler) WithGroup(n string) slog.Handler {
	return testContextHandler{h.Handler.WithGroup(n)}
}
