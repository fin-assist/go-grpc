package grpc_test

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/fin-assist/go-grpc/client"
	"github.com/fin-assist/go-grpc/reqctx"
	"github.com/fin-assist/go-grpc/server"
)

func Example_server() {
	srv, err := server.New(server.Options{}, server.Deps{Logger: slog.New(slog.DiscardHandler), Metrics: prometheus.NewRegistry(), Policy: server.Policy{"/example.Service/*": {Callers: []string{"assistant"}, Model: server.ModelSystem}}, Services: server.DevServiceVerifier{}, Users: server.DevUserVerifier{}, Tombstones: server.NoTombstones{}})
	if err != nil {
		panic(err)
	}
	// Dev verifiers are explicit here; production supplies go-identity implementations.
	// Register generated handlers on srv.GRPC(), then call srv.Serve(listener).
	srv.GRPC().Stop()
}
func Example_client() {
	conn, err := client.Dial("dns:///analytics:9090", client.Options{Peer: "analytics", Tokens: &client.FileToken{Path: "/run/service-account/token"}, Retry: []string{"example.Analytics/Get"}})
	if err != nil {
		panic(err)
	}
	defer func() { _ = conn.Close() }()
	// Pass conn to a generated client. Reads may use reqctx.RequestID from the parent RPC.
}
func Example_requestContext() {
	ctx := reqctx.WithRequestID(context.Background(), "trace-1")
	_ = reqctx.RequestID(ctx)
}
