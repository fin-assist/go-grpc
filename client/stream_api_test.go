package client_test

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/fin-assist/go-grpc/client"
	"github.com/fin-assist/go-grpc/reqctx"
)

type streamingPeer struct {
	testgrpc.UnimplementedTestServiceServer
	seen     chan metadata.MD
	deadline chan bool
}

func (p *streamingPeer) StreamingOutputCall(_ *testgrpc.StreamingOutputCallRequest, stream testgrpc.TestService_StreamingOutputCallServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	p.seen <- md
	_, has := stream.Context().Deadline()
	p.deadline <- has
	return stream.Send(&testgrpc.StreamingOutputCallResponse{})
}
func TestAPIStreamingPreservesCallerDeadlineAndIsolatesMetadata(t *testing.T) {
	p := &streamingPeer{seen: make(chan metadata.MD, 1), deadline: make(chan bool, 1)}
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	testgrpc.RegisterTestServiceServer(s, p)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := client.Dial("passthrough:///stream", client.Options{Peer: "peer", Tokens: client.DevToken("assistant"), DialOptions: []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) })}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithCancel(reqctx.WithRequestID(t.Context(), "stream-trace"))
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "private-user", "x-client-ip", "private-ip"))
	stream, err := testgrpc.NewTestServiceClient(conn).StreamingOutputCall(ctx, &testgrpc.StreamingOutputCallRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatal("stream did not finish", err)
	}
	md := <-p.seen
	if len(md.Get("authorization")) > 0 || len(md.Get("x-client-ip")) > 0 || md.Get("x-service-authorization")[0] != "Bearer dev:assistant" || md.Get("x-request-id")[0] != "stream-trace" {
		t.Fatal("stream metadata leaked", md)
	}
	if <-p.deadline {
		t.Fatal("stream received default unary deadline")
	}
}
func TestAPITokenFailurePreventsConsumerCall(t *testing.T) {
	for _, source := range []client.TokenSource{&client.FileToken{Path: filepath.Join(t.TempDir(), "missing")}, client.DevToken("")} {
		conn, err := client.Dial("passthrough:///unreachable", client.Options{Peer: "peer", Tokens: source})
		if err != nil {
			t.Fatal(err)
		}
		_, err = testgrpc.NewTestServiceClient(conn).EmptyCall(t.Context(), &testgrpc.Empty{})
		_ = conn.Close()
		if err == nil {
			t.Fatal("missing credentials accepted")
		}
	}
	for _, o := range []client.Options{{Peer: "peer"}, {Tokens: client.DevToken("svc")}, {Peer: "peer", Tokens: client.DevToken("svc"), Retry: []string{"/invalid"}}} {
		if conn, err := client.Dial("dns:///peer", o); err == nil {
			_ = conn.Close()
			t.Fatal("invalid client options accepted")
		}
	}
}
