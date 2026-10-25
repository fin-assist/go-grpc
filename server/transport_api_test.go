package server_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/fin-assist/go-grpc/client"
	"github.com/fin-assist/go-grpc/reqctx"
	"github.com/fin-assist/go-grpc/server"
)

type lockedLogs struct {
	sync.Mutex
	b strings.Builder
}

func (l *lockedLogs) Write(b []byte) (int, error) { l.Lock(); defer l.Unlock(); return l.b.Write(b) }
func (l *lockedLogs) String() string              { l.Lock(); defer l.Unlock(); return l.b.String() }

// A consumer registers its own protobuf method, not library-specific generated contracts.
func dynamicTransport(t *testing.T, message proto.Message) (*grpc.ClientConn, *lockedLogs, *server.Server) {
	t.Helper()
	logs := &lockedLogs{}
	reg := prometheus.NewRegistry()
	s, err := server.New(server.Options{}, server.Deps{
		Logger: slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if m, ok := a.Value.Any().(proto.Message); ok {
				a.Value = slog.StringValue(prototext.Format(m))
			}
			return a
		}})), Metrics: reg,
		Policy:   server.Policy{"/consumer.Validation/*": {Callers: []string{"assistant"}, Model: server.ModelSystem}},
		Services: server.DevServiceVerifier{}, Users: server.DevUserVerifier{}, Tombstones: server.NoTombstones{},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.GRPC().RegisterService(&grpc.ServiceDesc{ServiceName: "consumer.Validation", HandlerType: (*any)(nil), Methods: []grpc.MethodDesc{{MethodName: "Check", Handler: func(_ any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		m := dynamicpb.NewMessage(message.ProtoReflect().Descriptor())
		if err := decode(m); err != nil {
			return nil, err
		}
		return interceptor(ctx, m, &grpc.UnaryServerInfo{FullMethod: "/consumer.Validation/Check"}, func(ctx context.Context, req any) (any, error) { return &testgrpc.Empty{}, nil })
	}}}}, struct{}{})
	lis := bufconn.Listen(1 << 20)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(func() { s.GRPC().Stop(); _ = lis.Close() })
	m, err := client.NewMetrics(reg)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := client.Dial("passthrough:///validation", client.Options{Peer: "validation", Tokens: client.DevToken("assistant"), Metrics: m, DialOptions: []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) })}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, logs, s
}
func TestAPIActualServerClientValidationAndRedaction(t *testing.T) {
	for _, kind := range []string{"invalid", "broken", "nested-secrets"} {
		t.Run(kind, func(t *testing.T) {
			var m proto.Message
			switch kind {
			case "invalid":
				m = validationMessage(t)
			case "broken":
				m = brokenRuleMessage(t)
			default:
				m, _ = tokenMessage(t)
			}
			conn, logs, _ := dynamicTransport(t, m)
			ctx := reqctx.WithRequestID(t.Context(), "consumer-trace")
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "private-user", "x-other-token", "private-metadata", "trace-bin", "private-binary"))
			err := conn.Invoke(ctx, "/consumer.Validation/Check", m, &testgrpc.Empty{})
			want := ""
			if kind == "invalid" {
				want = "VALIDATION_ERROR"
			}
			if kind == "broken" {
				want = "INTERNAL_ERROR"
			}
			if server.Reason(err) != want {
				t.Fatalf("result %v reason %s", err, server.Reason(err))
			}
			if kind == "nested-secrets" {
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(logs.String(), "secret") || !strings.Contains(logs.String(), "visible") {
					t.Fatal("protobuf secrets leaked or visible values lost", logs.String())
				}
				if !strings.Contains(fmt.Sprint(m), "secret") {
					t.Fatal("logging mutated consumer message")
				}
			}
			if strings.Contains(logs.String(), "private-") {
				t.Fatal("outgoing metadata leaked", logs.String())
			}
		})
	}
}
func TestAPIConstructorPolicyAndShutdownContract(t *testing.T) {
	m, _ := tokenMessage(t)
	_, _, s := dynamicTransport(t, m)
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	s.SetServing(true)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.Shutdown(ctx, time.Hour)
	if _, err := server.New(server.Options{}, server.Deps{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
	for _, policy := range []server.Policy{{"/consumer.Validation/*": {Callers: []string{"assistant"}, Model: server.ModelUser, AllowDeleted: true}}, {"/consumer.Validation/Check": {Callers: []string{"assistant"}, Model: server.ModelSystem, AllowDeleted: true}}, {"invalid": {Callers: []string{"assistant"}, Model: server.ModelSystem}}, {"/consumer.Validation/*": {Model: server.ModelSystem}}, {"/consumer.Validation/*": {Callers: []string{"assistant"}}}} {
		srv, err := server.New(server.Options{Reflection: true}, server.Deps{Logger: slog.New(slog.DiscardHandler), Metrics: prometheus.NewRegistry(), Policy: policy, Services: server.DevServiceVerifier{}, Users: server.DevUserVerifier{}, Tombstones: server.NoTombstones{}})
		if err != nil {
			t.Fatal(err)
		}
		if srv.Check() == nil {
			t.Fatal("invalid policy accepted")
		}
		srv.GRPC().Stop()
	}
	if server.ModelUser.String() != "user" || server.ModelSystem.String() != "system" || server.ModelMixed.String() != "mixed" || server.ModelNoUser.String() != "no-user" || !strings.Contains(server.Model(99).String(), "99") {
		t.Fatal("model descriptions changed")
	}
	if server.Reason(errors.New("local")) != "" || server.Reason(io.EOF) != "" {
		t.Fatal("local error acquired public reason")
	}
}
