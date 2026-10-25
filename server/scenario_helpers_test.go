package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/fin-assist/go-grpc/reqctx"
	"github.com/fin-assist/go-grpc/server"
)

const (
	traceEcho   = "/consumer.Trace/Echo"
	traceUpload = "/consumer.Trace/Upload"
	traceHold   = "/consumer.Trace/Hold"
)

// traceService is a consumer service registered by hand, as a generated one would be: a
// unary Echo that reports the request ID it saw, a client stream Upload that counts the
// messages it accepted, and a server stream Hold that stays open until released.
type traceService struct {
	ids      chan string
	accepted chan int
	holding  chan struct{}
	release  chan struct{}
	holdEnd  chan error
	message  protoreflect.MessageDescriptor
}

func (s *traceService) desc() *grpc.ServiceDesc {
	return &grpc.ServiceDesc{
		ServiceName: "consumer.Trace",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "Echo", Handler: func(_ any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
			in := &testgrpc.Empty{}
			if err := decode(in); err != nil {
				return nil, err
			}
			return interceptor(ctx, in, &grpc.UnaryServerInfo{FullMethod: traceEcho}, func(ctx context.Context, _ any) (any, error) {
				s.ids <- reqctx.RequestID(ctx)
				return &testgrpc.Empty{}, nil
			})
		}}},
		Streams: []grpc.StreamDesc{
			{StreamName: "Upload", ClientStreams: true, Handler: func(_ any, ss grpc.ServerStream) error {
				n := 0
				for {
					if err := ss.RecvMsg(dynamicpb.NewMessage(s.message)); err != nil {
						s.accepted <- n
						if errors.Is(err, io.EOF) {
							return ss.SendMsg(&testgrpc.Empty{})
						}
						return err
					}
					n++
				}
			}},
			{StreamName: "Hold", ServerStreams: true, Handler: func(_ any, ss grpc.ServerStream) error {
				if err := ss.RecvMsg(&testgrpc.Empty{}); err != nil {
					return err
				}
				close(s.holding)
				var err error
				select {
				case <-s.release:
					err = ss.SendMsg(&testgrpc.Empty{})
				case <-ss.Context().Done():
					err = ss.Context().Err()
				}
				s.holdEnd <- err
				return err
			}},
		},
	}
}

// requestIDLogger adds the request ID from the context to every line, as the service
// logger (logx) does; the library itself never reads it into a log line.
type requestIDLogger struct{ slog.Handler }

func (h requestIDLogger) Handle(ctx context.Context, r slog.Record) error {
	if id := reqctx.RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

type scenario struct {
	srv  *server.Server
	svc  *traceService
	lis  *bufconn.Listener
	logs *lockedLogs
}

// startScenario serves traceService for the caller assistant (a system service); message
// is the type Upload receives.
func startScenario(t *testing.T, message protoreflect.MessageDescriptor) *scenario {
	t.Helper()
	logs := &lockedLogs{}
	srv, err := server.New(server.Options{}, server.Deps{
		Logger:   slog.New(requestIDLogger{slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})}),
		Metrics:  prometheus.NewRegistry(),
		Policy:   server.Policy{"/consumer.Trace/*": {Callers: []string{"assistant"}, Model: server.ModelSystem}},
		Services: server.DevServiceVerifier{}, Users: server.DevUserVerifier{}, Tombstones: server.NoTombstones{},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &traceService{ids: make(chan string, 1), accepted: make(chan int, 1), holding: make(chan struct{}),
		release: make(chan struct{}), holdEnd: make(chan error, 1), message: message}
	srv.GRPC().RegisterService(svc.desc(), struct{}{})
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.GRPC().Stop)
	return &scenario{srv: srv, svc: svc, lis: lis, logs: logs}
}

func (s *scenario) dialer() grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return s.lis.DialContext(ctx) })
}

// rawConn is a plain grpc-go client without the library's client interceptors.
func (s *scenario) rawConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///scenario", s.dialer(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

var (
	uploadDesc = &grpc.StreamDesc{StreamName: "Upload", ClientStreams: true}
	holdDesc   = &grpc.StreamDesc{StreamName: "Hold", ServerStreams: true}
)

// validRequest fills every rule of the validationMessage type.
func validRequest(t *testing.T) *dynamicpb.Message {
	t.Helper()
	m := dynamicpb.NewMessage(validationMessage(t).Descriptor())
	f := m.Descriptor().Fields()
	m.Set(f.ByName("name"), protoreflect.ValueOfString("statement"))
	m.Set(f.ByName("phone"), protoreflect.ValueOfString("+79001234567"))
	m.Set(f.ByName("id"), protoreflect.ValueOfString("upload-1"))
	return m
}
