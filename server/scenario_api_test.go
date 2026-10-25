package server_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"

	"github.com/fin-assist/go-grpc/client"
	"github.com/fin-assist/go-grpc/reqctx"
	"github.com/fin-assist/go-grpc/server"
)

// consumerConn dials the scenario server the way a service dials a peer: client.Dial with
// its token source, so x-service-authorization and x-request-id come from the library.
func consumerConn(t *testing.T, s *scenario) *grpc.ClientConn {
	t.Helper()
	conn, err := client.Dial("passthrough:///scenario", client.Options{Peer: "trace", Tokens: client.DevToken("assistant"), DialOptions: []grpc.DialOption{s.dialer()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// A service forwards the request ID of its own request with client.Dial; the next service
// keeps a valid one unchanged and replaces one that is too long.
func TestAPIRequestIDTravelsBetweenServices(t *testing.T) {
	for _, tc := range []struct{ name, id string }{
		{"valid", "0b6a7f6e-3c1d-4c43-9a51-8f0d2b1e4a77"},
		{"too long", strings.Repeat("L", 129)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startScenario(t, validationMessage(t).Descriptor())
			ctx := reqctx.WithRequestID(t.Context(), tc.id)
			if err := consumerConn(t, s).Invoke(ctx, traceEcho, &testgrpc.Empty{}, &testgrpc.Empty{}); err != nil {
				t.Fatal(err)
			}
			got := <-s.svc.ids
			if tc.name == "valid" && got != tc.id {
				t.Fatalf("request ID changed on the way: %q", got)
			}
			if tc.name == "too long" && (!generatedRequestID.MatchString(got) || strings.Contains(s.logs.String(), tc.id)) {
				t.Fatalf("too long request ID was kept or logged: %q", got)
			}
		})
	}
}

// A consumer that streams statement chunks gets VALIDATION_ERROR for an invalid chunk, and
// its handler never sees that chunk.
func TestAPIClientStreamMessagesAreValidated(t *testing.T) {
	s := startScenario(t, validationMessage(t).Descriptor())
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	st, err := consumerConn(t, s).NewStream(ctx, uploadDesc, traceUpload)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SendMsg(validRequest(t))
	_ = st.SendMsg(validationMessage(t))
	_ = st.CloseSend()
	err = st.RecvMsg(&testgrpc.Empty{})
	if status.Code(err) != codes.InvalidArgument || server.Reason(err) != "VALIDATION_ERROR" {
		t.Fatalf("got %v (reason %q)", err, server.Reason(err))
	}
	if n := <-s.svc.accepted; n != 1 {
		t.Fatalf("handler accepted %d messages, want 1", n)
	}
}

// Shutdown of the service lets a stream that ends before the shutdown deadline complete
// with OK for the consumer.
func TestAPIShutdownCompletesFinishingStream(t *testing.T) {
	s := startScenario(t, validationMessage(t).Descriptor())
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	st, err := consumerConn(t, s).NewStream(ctx, holdDesc, traceHold)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SendMsg(&testgrpc.Empty{}); err != nil {
		t.Fatal(err)
	}
	_ = st.CloseSend()
	<-s.svc.holding

	stopped := make(chan struct{})
	go func() { s.srv.Shutdown(ctx, 0); close(stopped) }()
	waitListenerClosed(t, s)
	close(s.svc.release)
	if err := st.RecvMsg(&testgrpc.Empty{}); err != nil {
		t.Fatalf("stream response: %v", err)
	}
	if err := st.RecvMsg(&testgrpc.Empty{}); !errors.Is(err, io.EOF) {
		t.Fatalf("stream end: %v, want OK", err)
	}
	<-stopped
	if ctx.Err() != nil {
		t.Fatal("Shutdown waited until its deadline instead of the stream")
	}
}
