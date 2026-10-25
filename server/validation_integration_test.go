package server_test

import (
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/fin-assist/go-grpc/reqctx"
	"github.com/fin-assist/go-grpc/server"
)

func TestIntegrationProtovalidateDetailsTravelOverTransport(t *testing.T) {
	message := validationMessage(t)
	conn, _, _ := dynamicTransport(t, message)
	err := conn.Invoke(t.Context(), "/consumer.Validation/Check", message, &testgrpc.Empty{})
	if server.Reason(err) != "VALIDATION_ERROR" {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, detail := range status.Convert(err).Details() {
		if br, ok := detail.(*errdetails.BadRequest); ok {
			for _, v := range br.FieldViolations {
				fields[v.Field] = v.Reason
			}
		}
	}
	for key, want := range map[string]string{"name": "OUT_OF_RANGE", "phone": "INVALID_FORMAT", "id": "REQUIRED"} {
		if fields[key] != want {
			t.Fatalf("field %s reason %s want %s", key, fields[key], want)
		}
	}
}
func TestIntegrationDynamicProtobufMaskingKeepsWireMessageIntact(t *testing.T) {
	message, _ := tokenMessage(t)
	before := proto.Clone(message)
	conn, logs, _ := dynamicTransport(t, message)
	err := conn.Invoke(reqctx.WithRequestID(t.Context(), "integration-mask"), "/consumer.Validation/Check", message, &testgrpc.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, message) {
		t.Fatal("logging altered caller payload")
	}
	if strings.Contains(logs.String(), "secret") || !strings.Contains(logs.String(), "visible") {
		t.Fatal("masked log changed public fields or retained token", logs.String())
	}
}
func TestIntegrationBrokenRuleIsServiceFailureOnWire(t *testing.T) {
	message := brokenRuleMessage(t)
	conn, logs, _ := dynamicTransport(t, message)
	err := conn.Invoke(t.Context(), "/consumer.Validation/Check", message, &testgrpc.Empty{})
	if server.Reason(err) != "INTERNAL_ERROR" || !strings.Contains(logs.String(), "protovalidate failed") {
		t.Fatalf("compiler error blamed on user: %v logs %s", err, logs.String())
	}
}
