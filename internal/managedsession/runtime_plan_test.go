package managedsession

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

// runtimePlanTestFence returns the attached-session fence used by runtime-plan fixtures.
func runtimePlanTestFence() ManagedPublicationFence {
	return ManagedPublicationFence{ProjectID: "project-orders", SessionID: "session-orders", SessionGeneration: 3}
}

// runtimePlanTestResponse returns one complete deterministic plan with App and service assignments.
func runtimePlanTestResponse() RuntimePlanResponse {
	fence := runtimePlanTestFence()
	return RuntimePlanResponse{
		SchemaVersion: SchemaVersion,
		Fence:         fence,
		Plan: RuntimePlan{
			Apps: []RuntimePlanApp{{
				ID:     "app",
				Active: true,
				Runtimes: []RuntimePlanRuntime{{
					ID: "http", BindHost: "127.0.0.10", BindPort: 43101, PublicURL: "https://orders.test",
					Routes: []RuntimePlanRoute{{Name: "health", Path: "/-/health"}, {Name: "ready", Path: "/-/ready"}},
				}},
			}},
			ServiceEndpoints: []RuntimePlanServiceEndpoint{{
				ID: "endpoint.database.primary.tcp", RequirementID: "requirement.database.primary",
				Consumers: []string{"app"}, PublishHost: "127.0.0.11", PublishPort: 43106,
				PublicHost: "mysql.orders.test", PublicPort: 3306,
			}},
		},
	}
}

// TestRuntimePlanRoundTripPreservesSemanticAssignments protects the cross-repository JSON shape.
func TestRuntimePlanRoundTripPreservesSemanticAssignments(t *testing.T) {
	response := runtimePlanTestResponse()
	payload, err := MarshalRuntimePlanResponse(response)
	if err != nil {
		t.Fatalf("MarshalRuntimePlanResponse() error = %v", err)
	}
	decoded, err := DecodeRuntimePlanResponse(payload)
	if err != nil {
		t.Fatalf("DecodeRuntimePlanResponse() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, response) {
		t.Fatalf("decoded response = %#v, want %#v", decoded, response)
	}

	request := RuntimePlanRequest{SchemaVersion: SchemaVersion, Fence: response.Fence, ActiveApps: []ActiveApp{{ID: "app", RuntimeIDs: []string{"http"}}}}
	requestPayload, err := MarshalRuntimePlanRequest(request)
	if err != nil {
		t.Fatalf("MarshalRuntimePlanRequest() error = %v", err)
	}
	if decodedRequest, err := DecodeRuntimePlanRequest(requestPayload); err != nil || !reflect.DeepEqual(decodedRequest, request) {
		t.Fatalf("DecodeRuntimePlanRequest() = %#v / %v, want %#v", decodedRequest, err, request)
	}
	if err := ValidateRuntimePlanCorrelation(request, response); err != nil {
		t.Fatalf("ValidateRuntimePlanCorrelation() error = %v", err)
	}
}

// TestRuntimePlanValidationRejectsUnsafeAssignments keeps private plan values out of process environments.
func TestRuntimePlanValidationRejectsUnsafeAssignments(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimePlanResponse)
		want   string
	}{
		{name: "non-loopback bind", mutate: func(response *RuntimePlanResponse) { response.Plan.Apps[0].Runtimes[0].BindHost = "0.0.0.0" }, want: "loopback"},
		{name: "low bind port", mutate: func(response *RuntimePlanResponse) { response.Plan.Apps[0].Runtimes[0].BindPort = 80 }, want: "bind port"},
		{name: "credential URL", mutate: func(response *RuntimePlanResponse) {
			response.Plan.Apps[0].Runtimes[0].PublicURL = "https://user:pass@orders.test"
		}, want: "credential"},
		{name: "unsorted routes", mutate: func(response *RuntimePlanResponse) {
			response.Plan.Apps[0].Runtimes[0].Routes[0], response.Plan.Apps[0].Runtimes[0].Routes[1] = response.Plan.Apps[0].Runtimes[0].Routes[1], response.Plan.Apps[0].Runtimes[0].Routes[0]
		}, want: "sorted"},
		{name: "invalid upstream", mutate: func(response *RuntimePlanResponse) { response.Plan.ServiceEndpoints[0].PublishHost = "::1" }, want: "loopback"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := runtimePlanTestResponse()
			test.mutate(&response)
			if err := response.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RuntimePlanResponse.Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

// TestRuntimePlanCorrelationRejectsMissingOrForeignApps keeps Harbor from silently changing the active topology.
func TestRuntimePlanCorrelationRejectsMissingOrForeignApps(t *testing.T) {
	response := runtimePlanTestResponse()
	request := RuntimePlanRequest{SchemaVersion: SchemaVersion, Fence: response.Fence, ActiveApps: []ActiveApp{{ID: "app", RuntimeIDs: []string{"http"}}, {ID: "worker", RuntimeIDs: []string{}}}}
	if err := ValidateRuntimePlanCorrelation(request, response); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing App correlation error = %v", err)
	}
	request.ActiveApps = []ActiveApp{{ID: "other", RuntimeIDs: []string{}}}
	if err := ValidateRuntimePlanCorrelation(request, response); err == nil || !strings.Contains(err.Error(), "unrequested") {
		t.Fatalf("foreign App correlation error = %v", err)
	}
	request.ActiveApps = []ActiveApp{{ID: "app", RuntimeIDs: []string{"worker"}}}
	if err := ValidateRuntimePlanCorrelation(request, response); err == nil || !strings.Contains(err.Error(), "runtime set") {
		t.Fatalf("runtime correlation error = %v", err)
	}
	request.ActiveApps = []ActiveApp{{ID: "app", RuntimeIDs: []string{"http"}}}
	response.Plan.Apps[0].Active = false
	if err := ValidateRuntimePlanCorrelation(request, response); err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("inactive App correlation error = %v", err)
	}
}

// TestRuntimePlanFixtureUsesCanonicalPrivateAddress documents the required address family for future allocators.
func TestRuntimePlanFixtureUsesCanonicalPrivateAddress(t *testing.T) {
	address := netip.MustParseAddr("127.0.0.10")
	if !address.Is4() || !address.IsLoopback() || address != address.Unmap() {
		t.Fatalf("fixture address = %s, want canonical IPv4 loopback", address)
	}
}
