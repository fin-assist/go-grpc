package server_test

import (
	"context"
	"testing"

	"github.com/fin-assist/contracts/gen/go/finerr"
	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"

	"github.com/fin-assist/go-grpc/server"
)

type revokedUser struct{}

func (revokedUser) VerifyUser(context.Context, string) (string, error) {
	return "", finerr.TokenRevoked()
}
func TestAPICleanupPolicyIsStaticAndStillAuthenticates(t *testing.T) {
	for _, tc := range []struct {
		name, caller, user string
		allow, revoked     bool
		code               codes.Code
	}{
		{"default denies", "bank-bff", "gone", false, false, codes.Unauthenticated}, {"explicit cleanup", "bank-bff", "gone", true, false, codes.OK}, {"missing user", "bank-bff", "", true, false, codes.Unauthenticated}, {"foreign caller", "analytics", "gone", true, false, codes.PermissionDenied}, {"revoked token", "bank-bff", "gone", true, true, codes.Unauthenticated}, {"store unavailable", "bank-bff", "broken", true, false, codes.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := policy()
			r := p["/grpc.testing.TestService/*"]
			r.AllowDeleted = tc.allow
			p[emptyCall] = r
			e := start(t, p, func(d *server.Deps) {
				if tc.revoked {
					d.Users = revokedUser{}
				}
			})
			_, err := testgrpc.NewTestServiceClient(e.conn).EmptyCall(call(t, tc.caller, tc.user, "x-allow-deleted", "true"), &testgrpc.Empty{})
			if status.Code(err) != tc.code {
				t.Fatal(err)
			}
			if tc.code != codes.OK {
				select {
				case <-e.svc.got:
					t.Fatal("refused caller reached handler")
				default:
				}
			}
		})
	}
}
