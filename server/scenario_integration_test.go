package server_test

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/fin-assist/go-grpc/server"
)

var generatedRequestID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func systemCall(t *testing.T, kv ...string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(append([]string{"x-service-authorization", "Bearer dev:assistant"}, kv...)...))
}

// grpc-api.md 4: x-request-id passes through unchanged; a value of unexpected shape or
// length is replaced by a generated one and never reaches the log.
func TestIntegrationRequestIDOverTransport(t *testing.T) {
	limit := strings.Repeat("r", 128)
	tooLong := strings.Repeat("L", 129)
	for _, tc := range []struct {
		name   string
		values []string
		want   string // "" means a generated ID
		logged string // the masked x-request-id of the debug metadata, when it matters
	}{
		{"uuid", []string{"0b6a7f6e-3c1d-4c43-9a51-8f0d2b1e4a77"}, "0b6a7f6e-3c1d-4c43-9a51-8f0d2b1e4a77", ""},
		{"128 characters", []string{limit}, limit, ""},
		{"129 characters", []string{tooLong}, "", ""},
		{"space", []string{"bad id"}, "", ""},
		{"absent", nil, "", ""},
		{"two values", []string{"first-id", "second-id"}, "first-id", "first-id,second-id"},
		{"valid first, rejected second", []string{"first-id", "bad id"}, "first-id", "first-id,(rejected)"},
		{"rejected first, valid second", []string{"bad id", "second-id"}, "", "(rejected),second-id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startScenario(t, validationMessage(t).Descriptor())
			var kv []string
			for _, v := range tc.values {
				kv = append(kv, "x-request-id", v)
			}
			if err := s.rawConn(t).Invoke(systemCall(t, kv...), traceEcho, &testgrpc.Empty{}, &testgrpc.Empty{}); err != nil {
				t.Fatal(err)
			}
			got := <-s.svc.ids
			if tc.want != "" && got != tc.want || tc.want == "" && (!generatedRequestID.MatchString(got) || strings.Contains(strings.Join(tc.values, ","), got)) {
				t.Fatalf("handler saw request ID %q, want %q", got, tc.want)
			}
			if !strings.Contains(s.logs.String(), `"request_id":"`+got+`"`) {
				t.Fatalf("call line lacks the request ID %q: %s", got, s.logs.String())
			}
			if tc.logged != "" && !strings.Contains(s.logs.String(), `"x-request-id":"`+tc.logged+`"`) {
				t.Fatalf("debug metadata lacks x-request-id %q: %s", tc.logged, s.logs.String())
			}
			for _, rejected := range []string{tooLong, "bad id"} {
				if strings.Contains(s.logs.String(), rejected) {
					t.Fatalf("a rejected request ID reached the log: %s", s.logs.String())
				}
			}
		})
	}
}

// Every message of a client stream is validated before the handler sees it: the first
// valid message is accepted, the second one ends the stream with VALIDATION_ERROR.
func TestIntegrationClientStreamValidatesEveryMessage(t *testing.T) {
	s := startScenario(t, validationMessage(t).Descriptor())
	conn := s.rawConn(t)

	st, err := conn.NewStream(systemCall(t), uploadDesc, traceUpload)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SendMsg(validRequest(t))
	_ = st.SendMsg(validationMessage(t))
	_ = st.CloseSend()
	err = st.RecvMsg(&testgrpc.Empty{})
	if status.Code(err) != codes.InvalidArgument || server.Reason(err) != "VALIDATION_ERROR" {
		t.Fatalf("invalid stream message: %v (reason %q)", err, server.Reason(err))
	}
	var violations int
	for _, d := range status.Convert(err).Details() {
		if br, ok := d.(*errdetails.BadRequest); ok {
			violations += len(br.GetFieldViolations())
		}
	}
	if violations != 3 {
		t.Fatalf("BadRequest carries %d violations, want 3", violations)
	}
	if n := <-s.svc.accepted; n != 1 {
		t.Fatalf("handler accepted %d messages, want only the valid one", n)
	}

	st, err = conn.NewStream(systemCall(t), uploadDesc, traceUpload)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := st.SendMsg(validRequest(t)); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.CloseSend()
	if err := st.RecvMsg(&testgrpc.Empty{}); err != nil {
		t.Fatalf("valid stream: %v", err)
	}
	if n := <-s.svc.accepted; n != 2 {
		t.Fatalf("handler accepted %d messages, want 2", n)
	}
}

// waitListenerClosed returns once GracefulStop closed the listener: new connections fail.
func waitListenerClosed(t *testing.T, s *scenario) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for time.Now().Before(deadline) {
		c, err := s.lis.DialContext(t.Context())
		if err != nil {
			return
		}
		_ = c.Close()
		<-tick.C
	}
	t.Fatal("Shutdown did not start the graceful stop")
}

// waitHolding waits until the Hold handler runs, or fails when the call ends first.
func waitHolding(t *testing.T, s *scenario, call context.Context) {
	t.Helper()
	select {
	case <-s.svc.holding:
	case <-call.Done():
		t.Fatalf("Hold handler never started: %v", call.Err())
	}
}

// assertStillStopping fails when Shutdown returns while the handler is still held. The
// window bounds a negative check; it does not synchronise anything.
func assertStillStopping(t *testing.T, stopped <-chan struct{}) {
	t.Helper()
	select {
	case <-stopped:
		t.Fatal("Shutdown returned while a stream was still running")
	case <-time.After(200 * time.Millisecond):
	}
}

// A stream that finishes on its own before the shutdown context ends is not cut: Shutdown
// waits for it and the caller gets its response and OK, not a transport error.
func TestIntegrationShutdownLetsActiveStreamFinish(t *testing.T) {
	s := startScenario(t, validationMessage(t).Descriptor())
	s.srv.SetServing(true)
	st, err := s.rawConn(t).NewStream(systemCall(t), holdDesc, traceHold)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SendMsg(&testgrpc.Empty{}); err != nil {
		t.Fatal(err)
	}
	_ = st.CloseSend()
	waitHolding(t, s, st.Context())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	go func() { s.srv.Shutdown(ctx, 0); close(stopped) }()
	waitListenerClosed(t, s)
	assertStillStopping(t, stopped)
	close(s.svc.release)
	if err := <-s.svc.holdEnd; err != nil {
		t.Fatalf("handler was cut: %v", err)
	}
	resp := &testgrpc.Empty{}
	if err := st.RecvMsg(resp); err != nil || !proto.Equal(resp, &testgrpc.Empty{}) {
		t.Fatalf("stream response: %v", err)
	}
	if err := st.RecvMsg(resp); !errors.Is(err, io.EOF) {
		t.Fatalf("stream end: %v, want OK", err)
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("Shutdown did not return after the stream finished")
	}
	if ctx.Err() != nil {
		t.Fatal("Shutdown waited until its deadline instead of the stream")
	}
}
