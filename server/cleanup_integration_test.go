package server

import (
	"testing"

	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"
)

func TestIntegrationCleanupRouteStillRejectsUntrustedMetadata(t *testing.T) {
	p := policy()
	r := p["/grpc.testing.TestService/*"]
	r.AllowDeleted = true
	p[emptyCall] = r
	e := start(t, p)
	c := testgrpc.NewTestServiceClient(e.conn)
	if _, err := c.EmptyCall(call(t, "bank-bff", "gone"), &testgrpc.Empty{}); err != nil {
		t.Fatal(err)
	}
	<-e.svc.got
	// Exact-method mixed rule overrides the wildcard cleanup exception.
	if _, err := c.UnaryCall(call(t, "bank-bff", "gone", "x-allow-deleted", "true"), &testgrpc.SimpleRequest{}); status.Code(err) != codes.Unauthenticated || Reason(err) != "TOKEN_REVOKED" {
		t.Fatalf("metadata overrode static policy: %v", err)
	}
	select {
	case <-e.svc.got:
		t.Fatal("denied route reached handler")
	default:
	}
}
