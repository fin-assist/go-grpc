package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
func TestUnitDependencyLogsDoNotContainRawErrors(t *testing.T) {
	for _, serviceFailure := range []bool{true, false} {
		s, logs := isolatedServer(t, policy(), func(d *Deps) {
			if serviceFailure {
				d.Services = failingService{}
			} else {
				d.Tombstones = failingTombstone{}
			}
		})
		_, err := s.authorize(incoming(t, "bank-bff", "u1"), emptyCall)
		want := codes.Unavailable
		if serviceFailure {
			want = codes.Unauthenticated
		}
		if status.Code(err) != want {
			t.Fatal(err)
		}
		if strings.Contains(logs.String(), "private-") || strings.Contains(logs.String(), "SELECT") {
			t.Fatal("dependency cause leaked", logs.String())
		}
	}
}
func TestUnitStreamingErrorLogsDoNotContainRawCause(t *testing.T) {
	s, logs := isolatedServer(t, policy())
	err := s.stream(nil, &isolatedStream{ctx: incoming(t, "user", "")}, &grpc.StreamServerInfo{FullMethod: streamOut}, func(any, grpc.ServerStream) error { return errors.New(sensitiveCause) })
	if status.Code(err) != codes.Internal {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "private-") || strings.Contains(logs.String(), "SELECT") {
		t.Fatal("stream cause leaked", logs.String())
	}
}
