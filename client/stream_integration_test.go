package client

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/test/bufconn"
)

type cancellablePeer struct {
	testgrpc.UnimplementedTestServiceServer
	started, stopped chan struct{}
}

func (p *cancellablePeer) StreamingOutputCall(_ *testgrpc.StreamingOutputCallRequest, s testgrpc.TestService_StreamingOutputCallServer) error {
	close(p.started)
	<-s.Context().Done()
	close(p.stopped)
	return s.Context().Err()
}
func TestIntegrationClientStreamCancellationReachesTransport(t *testing.T) {
	p := &cancellablePeer{started: make(chan struct{}), stopped: make(chan struct{})}
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	testgrpc.RegisterTestServiceServer(s, p)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	c, err := Dial("passthrough:///cancellation", Options{Peer: "peer", Tokens: DevToken("assistant"), DialOptions: []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) })}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream, err := testgrpc.NewTestServiceClient(c).StreamingOutputCall(ctx, &testgrpc.StreamingOutputCallRequest{})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = stream.Recv() }()
	select {
	case <-p.started:
	case <-time.After(3 * time.Second):
		t.Fatal("stream not admitted")
	}
	cancel()
	select {
	case <-p.stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not reach peer")
	}
}
