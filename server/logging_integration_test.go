package server

import (
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"
)

func TestIntegrationAdmissionErrorsRetainOnlySafeLogFields(t *testing.T) {
	for _, serviceFailure := range []bool{true, false} {
		e := start(t, policy(), func(d *Deps) {
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
		logs := e.logs.String()
		if strings.Contains(logs, "SELECT") || strings.Contains(logs, "private-") {
			t.Fatal("admission raw error leaked", logs)
		}
		if !strings.Contains(logs, `"method"`) {
			t.Fatal("safe method context missing", logs)
		}
	}
}
