package server

import (
	"context"
	"testing"
	"time"

	"github.com/fin-assist/contracts/gen/go/finerr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type revokedUser struct{}

func (revokedUser) VerifyUser(context.Context, string) (string, error) {
	return "", finerr.TokenRevoked()
}
func TestUnitCleanupNeverBypassesIdentity(t *testing.T) {
	p := policy()
	r := p["/grpc.testing.TestService/*"]
	r.AllowDeleted = true
	p[emptyCall] = r
	for _, tc := range []struct {
		caller, user string
		revoked      bool
		code         codes.Code
		reason       string
	}{
		{"bank-bff", "gone", false, codes.OK, ""}, {"bank-bff", "", false, codes.Unauthenticated, "INVALID_TOKEN"}, {"analytics", "gone", false, codes.PermissionDenied, ""}, {"", "gone", false, codes.Unauthenticated, ""}, {"bank-bff", "gone", true, codes.Unauthenticated, "TOKEN_REVOKED"}, {"bank-bff", "broken", false, codes.Unavailable, ""},
	} {
		s, _ := isolatedServer(t, p, func(d *Deps) {
			if tc.revoked {
				d.Users = revokedUser{}
			}
		})
		_, err := s.authorize(incoming(t, tc.caller, tc.user), emptyCall)
		if status.Code(err) != tc.code || Reason(err) != tc.reason {
			t.Fatalf("caller=%q user=%q revoked=%v err=%v", tc.caller, tc.user, tc.revoked, err)
		}
	}
}
func TestUnitReadinessAndLifecycleWithoutTransport(t *testing.T) {
	s, _ := isolatedServer(t, policy())
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	s.SetServing(true)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.Shutdown(ctx, time.Hour)
	s.SetServing(false)
	for _, m := range []Model{ModelUser, ModelSystem, ModelMixed, ModelNoUser, 99} {
		if m.String() == "" {
			t.Fatal("missing model text")
		}
	}
}
