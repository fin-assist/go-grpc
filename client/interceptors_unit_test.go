package client

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/fin-assist/go-grpc/reqctx"
)

type failingToken struct{}

func (failingToken) Token(context.Context) (string, error) { return "", io.ErrUnexpectedEOF }
func TestUnitOutgoingAdmissionDeadlineAndMetadata(t *testing.T) {
	m, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range []bool{false, true} {
		o := Options{Peer: "peer", Tokens: DevToken("assistant"), DefaultTimeout: time.Second, Metrics: m}
		if broken {
			o.Tokens = failingToken{}
		}
		base := reqctx.WithRequestID(t.Context(), "request-1")
		base = metadata.NewOutgoingContext(base, metadata.Pairs("authorization", "private-user", "x-client-ip", "private-ip", "x-service-authorization", "foreign"))
		invoked := false
		err := o.unary(base, "/svc/Read", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			invoked = true
			md, _ := metadata.FromOutgoingContext(ctx)
			if len(md) != 2 || md.Get("authorization") != nil || md.Get("x-client-ip") != nil || md.Get("x-service-authorization")[0] != "Bearer dev:assistant" || md.Get("x-request-id")[0] != "request-1" {
				t.Errorf("metadata leaked: %v", md)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Error("unbounded call")
			}
			return context.Canceled
		})
		if broken {
			if invoked || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("token failure not terminal: %v", err)
			}
		} else if !invoked || !errors.Is(err, context.Canceled) {
			t.Fatalf("invoker result lost %v", err)
		}
	}
	o := Options{DefaultTimeout: time.Second}
	ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
	defer cancel()
	derived, stop := o.deadline(ctx)
	defer stop()
	a, _ := ctx.Deadline()
	b, _ := derived.Deadline()
	if a != b {
		t.Fatal("explicit deadline changed")
	}
}
func TestUnitStreamMetadataAndFailure(t *testing.T) {
	for _, broken := range []bool{false, true} {
		o := Options{Tokens: DevToken("assistant")}
		if broken {
			o.Tokens = failingToken{}
		}
		invoked := false
		_, err := o.stream(reqctx.WithRequestID(t.Context(), "s1"), nil, nil, "/svc/Watch", func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			invoked = true
			if _, ok := ctx.Deadline(); ok {
				t.Error("stream acquired default deadline")
			}
			md, _ := metadata.FromOutgoingContext(ctx)
			if md.Get("x-request-id")[0] != "s1" {
				t.Error("stream request id missing")
			}
			return nil, io.EOF
		})
		if broken {
			if invoked || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("stream token error bypassed")
			}
		} else if !invoked || !errors.Is(err, io.EOF) {
			t.Fatal("streamer result lost")
		}
	}
}
func TestUnitMetricRegistrationAndOptionalMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	if _, err := NewMetrics(reg); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetrics(reg); err == nil {
		t.Fatal("duplicate metrics silently reused")
	}
	var m *Metrics
	m.observe("peer", "/s/m", nil, time.Now())
	cfg, err := serviceConfig([]string{"s.Service/Read", "s.Other"})
	if err != nil || cfg == "" {
		t.Fatal("valid retry policy refused", err)
	}
}
