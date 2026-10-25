package server_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"

	"github.com/fin-assist/go-grpc/server"
)

const sensitiveCause = "SELECT private_user_secret; postgres://private-dsn token=private-token"

type failingService struct{}

func (failingService) VerifyService(context.Context, string) (string, error) {
	return "", errors.New(sensitiveCause)
}

type failingTombstone struct{}

func (failingTombstone) IsDeleted(context.Context, string) (bool, error) {
	return false, errors.New(sensitiveCause)
}
func TestAPIConsumerLoggerNeverReceivesDependencySecrets(t *testing.T) {
	for _, serviceFailure := range []bool{true, false} {
		e := start(t, policy(), func(d *server.Deps) {
			if serviceFailure {
				d.Services = failingService{}
			} else {
				d.Tombstones = failingTombstone{}
			}
		})
		_, err := testgrpc.NewTestServiceClient(e.conn).EmptyCall(call(t, "bank-bff", "u1"), &testgrpc.Empty{})
		want := codes.Unavailable
		if serviceFailure {
			want = codes.Unauthenticated
		}
		if status.Code(err) != want {
			t.Fatal(err)
		}
		if strings.Contains(e.logs.String(), "private-") || strings.Contains(e.logs.String(), "SELECT") {
			t.Fatal("dependency cause leaked", e.logs.String())
		}
		select {
		case <-e.svc.got:
			t.Fatal("refused call reached consumer handler")
		default:
		}
	}
}
