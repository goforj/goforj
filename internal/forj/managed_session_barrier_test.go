package forj

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/managedsession"
	"github.com/goforj/goforj/project"
)

// reconnectingManagedBarrierFixture adds lifecycle closure accounting to the barrier recorder.
type reconnectingManagedBarrierFixture struct {
	recordingManagedBarrierClient
	closeCalls int
}

// Close records replacement cleanup for the reconnecting session fixture.
func (client *reconnectingManagedBarrierFixture) Close() error {
	client.closeCalls++
	return nil
}

// managedBarrierTestLaunchContext returns one valid inherited authority for reconnect tests.
func managedBarrierTestLaunchContext() managedsession.LaunchContext {
	return managedsession.LaunchContext{
		SchemaVersion:             managedsession.ManagedLaunchContextSchemaVersion,
		ProjectID:                 "project-orders",
		SessionID:                 "session-orders",
		ProjectRoot:               "/tmp/orders",
		ExpectedSessionGeneration: 1,
		DescriptorDigest:          strings.Repeat("a", 64),
		EndpointReference:         "/tmp/harbord.sock",
		Owner:                     managedsession.SessionOwnerHarbor,
		Ticket:                    strings.Repeat("b", 32),
	}
}

// recordingManagedBarrierClient records the complete replacement and barrier requests used by the retry helper.
type recordingManagedBarrierClient struct {
	replaceRequests []managedsession.ReplacePublicationsRequest
	barrierRequests []managedsession.BarrierRequest
	barrierReplies  []managedsession.BarrierResponse
	replaceErrors   []error
	barrierErrors   []error
	runtimeRequests []managedsession.RuntimePlanRequest
	runtimeReplies  []managedsession.RuntimePlanResponse
	runtimeErrors   []error
	peer            managedsession.Peer
}

// ReplacePublications records one client-side replacement and returns the next configured result.
func (client *recordingManagedBarrierClient) ReplacePublications(_ context.Context, request managedsession.ReplacePublicationsRequest) (managedsession.ReplacePublicationsResponse, error) {
	client.replaceRequests = append(client.replaceRequests, request)
	if len(client.replaceErrors) > 0 {
		err := client.replaceErrors[0]
		client.replaceErrors = client.replaceErrors[1:]
		return managedsession.ReplacePublicationsResponse{}, err
	}
	return managedsession.ReplacePublicationsResponse{
		SchemaVersion:    managedsession.SchemaVersion,
		Fence:            request.Fence,
		Accepted:         true,
		PublicationCount: uint16(len(request.Publications)),
	}, nil
}

// Barrier records one lifecycle barrier and returns the next configured result.
func (client *recordingManagedBarrierClient) Barrier(_ context.Context, request managedsession.BarrierRequest) (managedsession.BarrierResponse, error) {
	client.barrierRequests = append(client.barrierRequests, request)
	if len(client.barrierErrors) > 0 {
		err := client.barrierErrors[0]
		client.barrierErrors = client.barrierErrors[1:]
		return managedsession.BarrierResponse{}, err
	}
	if len(client.barrierReplies) == 0 {
		return managedsession.BarrierResponse{SchemaVersion: managedsession.SchemaVersion, Fence: request.Fence, Phase: request.Phase, Acknowledged: true}, nil
	}
	reply := client.barrierReplies[0]
	client.barrierReplies = client.barrierReplies[1:]
	return reply, nil
}

// RuntimePlan records one optional assignment request and returns the next configured response.
func (client *recordingManagedBarrierClient) RuntimePlan(_ context.Context, request managedsession.RuntimePlanRequest) (managedsession.RuntimePlanResponse, error) {
	client.runtimeRequests = append(client.runtimeRequests, request)
	if len(client.runtimeErrors) > 0 {
		err := client.runtimeErrors[0]
		client.runtimeErrors = client.runtimeErrors[1:]
		return managedsession.RuntimePlanResponse{}, err
	}
	if len(client.runtimeReplies) == 0 {
		return managedsession.RuntimePlanResponse{
			SchemaVersion: managedsession.SchemaVersion,
			Fence:         request.Fence,
			Plan: managedsession.RuntimePlan{
				Apps:             []managedsession.RuntimePlanApp{},
				ServiceEndpoints: []managedsession.RuntimePlanServiceEndpoint{},
			},
		}, nil
	}
	reply := client.runtimeReplies[0]
	client.runtimeReplies = client.runtimeReplies[1:]
	return reply, nil
}

// Peer returns the negotiated daemon capabilities used by the optional plan gate.
func (client *recordingManagedBarrierClient) Peer() managedsession.Peer {
	peer := client.peer
	peer.Capabilities = append([]managedsession.Capability(nil), peer.Capabilities...)
	return peer
}

// managedBarrierTestRegistration returns a valid attached-session fence for helper tests.
func managedBarrierTestRegistration(t *testing.T) managedsession.RegisterResponse {
	t.Helper()
	return managedsession.RegisterResponse{
		SchemaVersion: managedsession.SchemaVersion,
		Fence: managedsession.ManagedPublicationFence{
			ProjectID:         "project-orders",
			SessionID:         "session-orders",
			SessionGeneration: 2,
		},
		AttachmentTicket: "attachment-ticket-orders",
	}
}

// TestWaitForManagedComposeBarrierRetriesUntilHarborAcknowledges verifies the helper does not block watcher startup on a temporary route race.
func TestWaitForManagedComposeBarrierRetriesUntilHarborAcknowledges(t *testing.T) {
	registration := managedBarrierTestRegistration(t)
	client := &recordingManagedBarrierClient{
		barrierReplies: []managedsession.BarrierResponse{
			{SchemaVersion: managedsession.SchemaVersion, Fence: registration.Fence, Phase: managedsession.BarrierPhaseCompose},
			{SchemaVersion: managedsession.SchemaVersion, Fence: registration.Fence, Phase: managedsession.BarrierPhaseCompose, Acknowledged: true},
		},
	}
	if err := waitForManagedComposeBarrier(t.Context(), client, registration, "orders-dev"); err != nil {
		t.Fatalf("waitForManagedComposeBarrier() error = %v", err)
	}
	if len(client.replaceRequests) != 1 || len(client.replaceRequests[0].Publications) != 0 {
		t.Fatalf("replacement requests = %#v, want one empty replacement", client.replaceRequests)
	}
	if len(client.barrierRequests) != 2 || client.barrierRequests[0].AcceptedProjectIdentity != "orders-dev" {
		t.Fatalf("barrier requests = %#v, want two requests retaining the accepted identity", client.barrierRequests)
	}
}

// TestWaitForManagedComposeBarrierRetriesUnavailableCalls verifies only the explicit unavailable category is retried.
func TestWaitForManagedComposeBarrierRetriesUnavailableCalls(t *testing.T) {
	registration := managedBarrierTestRegistration(t)
	client := &recordingManagedBarrierClient{
		replaceErrors: []error{
			managedsession.RemoteError{Failure: managedsession.WireError{Code: managedsession.ErrorCodeUnavailable, Message: "temporary", Retryable: true}},
		},
	}
	if err := waitForManagedComposeBarrier(t.Context(), client, registration, "orders"); err != nil {
		t.Fatalf("waitForManagedComposeBarrier() error = %v", err)
	}
	if len(client.replaceRequests) != 2 || len(client.barrierRequests) != 1 {
		t.Fatalf("calls = replacements %d, barriers %d; want two replacements and one barrier", len(client.replaceRequests), len(client.barrierRequests))
	}
}

// TestWaitForManagedComposeBarrierHonorsCallerDeadline proves a temporary Harbor outage cannot outlive the lifecycle context.
func TestWaitForManagedComposeBarrierHonorsCallerDeadline(t *testing.T) {
	registration := managedBarrierTestRegistration(t)
	client := &recordingManagedBarrierClient{
		barrierReplies: make([]managedsession.BarrierResponse, 8),
	}
	for index := range client.barrierReplies {
		client.barrierReplies[index] = managedsession.BarrierResponse{
			SchemaVersion: managedsession.SchemaVersion,
			Fence:         registration.Fence,
			Phase:         managedsession.BarrierPhaseCompose,
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	err := waitForManagedComposeBarrier(ctx, client, registration, "orders")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitForManagedComposeBarrier() error = %v, want deadline exceeded", err)
	}
	if len(client.barrierRequests) < 2 {
		t.Fatalf("barrier requests = %d, want retry before deadline", len(client.barrierRequests))
	}
}

// TestManagedComposeBarrierContextAddsDefaultDeadline proves an unbounded dev context receives the startup safety budget.
func TestManagedComposeBarrierContextAddsDefaultDeadline(t *testing.T) {
	before := time.Now()
	ctx, cancel := managedComposeBarrierContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("managedComposeBarrierContext() returned no deadline")
	}
	if deadline.Before(before.Add(managedComposeBarrierTimeout-time.Second)) || deadline.After(before.Add(managedComposeBarrierTimeout+time.Second)) {
		t.Fatalf("managed barrier deadline = %v, want about %v", deadline, before.Add(managedComposeBarrierTimeout))
	}
}

// TestManagedSessionHeartbeatStopsAfterConsecutiveFailures proves route refresh cannot issue unbounded IPC retries.
func TestManagedSessionHeartbeatStopsAfterConsecutiveFailures(t *testing.T) {
	ticks := make(chan time.Time, managedHeartbeatFailureLimit)
	for index := 0; index < managedHeartbeatFailureLimit; index++ {
		ticks <- time.Now()
	}
	barrierCalls := 0
	done := make(chan struct{})
	go func() {
		runManagedSessionHeartbeatLoop(context.Background(), ticks, io.Discard, func(context.Context) error {
			barrierCalls++
			return errors.New("managed route is unavailable")
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runManagedSessionHeartbeatLoop() did not stop after its failure budget")
	}
	if barrierCalls != managedHeartbeatFailureLimit {
		t.Fatalf("barrier calls = %d, want %d", barrierCalls, managedHeartbeatFailureLimit)
	}
}

// TestManagedSessionHeartbeatResetsFailuresAfterSuccess keeps one transient route refresh failure from disabling future refreshes.
func TestManagedSessionHeartbeatResetsFailuresAfterSuccess(t *testing.T) {
	wantCalls := managedHeartbeatFailureLimit + 2
	ticks := make(chan time.Time, wantCalls)
	for index := 0; index < wantCalls; index++ {
		ticks <- time.Now()
	}
	barrierCalls := 0
	done := make(chan struct{})
	go func() {
		runManagedSessionHeartbeatLoop(context.Background(), ticks, io.Discard, func(context.Context) error {
			barrierCalls++
			if barrierCalls == 2 {
				return nil
			}
			return errors.New("managed route is unavailable")
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runManagedSessionHeartbeatLoop() did not stop after the reset failure budget")
	}
	if barrierCalls != wantCalls {
		t.Fatalf("barrier calls = %d, want %d", barrierCalls, wantCalls)
	}
}

// TestReconnectingManagedSessionRetriesPublicationAfterDisconnect proves a lost stream is replayed before the
// barrier helper gives up, while a successful remote response remains the only terminal success condition.
func TestReconnectingManagedSessionRetriesPublicationAfterDisconnect(t *testing.T) {
	registration := managedBarrierTestRegistration(t)
	initial := &reconnectingManagedBarrierFixture{recordingManagedBarrierClient: recordingManagedBarrierClient{
		replaceErrors: []error{managedsession.ErrDisconnected},
	}}
	replacement := &reconnectingManagedBarrierFixture{}
	reconnectCalls := 0
	connection, err := newReconnectingManagedSession(
		initial,
		registration,
		managedBarrierTestLaunchContext(),
		func(context.Context, managedsession.LaunchContext) (managedSessionClient, managedsession.RegisterResponse, error) {
			reconnectCalls++
			return replacement, registration, nil
		},
	)
	if err != nil {
		t.Fatalf("newReconnectingManagedSession() error = %v", err)
	}
	if err := waitForManagedComposeBarrier(t.Context(), connection, registration, "orders"); err != nil {
		t.Fatalf("waitForManagedComposeBarrier() error = %v", err)
	}
	if reconnectCalls != 1 || initial.closeCalls != 1 || len(initial.replaceRequests) != 1 || len(replacement.replaceRequests) != 1 {
		t.Fatalf("reconnect calls=%d initial closes=%d initial replacements=%d replacement replacements=%d", reconnectCalls, initial.closeCalls, len(initial.replaceRequests), len(replacement.replaceRequests))
	}
}

// TestReconnectingManagedSessionRetriesBarrierAfterDisconnect proves a barrier loss resets only the connection and
// preserves the exact session fence on the replayed request.
func TestReconnectingManagedSessionRetriesBarrierAfterDisconnect(t *testing.T) {
	registration := managedBarrierTestRegistration(t)
	initial := &reconnectingManagedBarrierFixture{recordingManagedBarrierClient: recordingManagedBarrierClient{
		barrierErrors: []error{managedsession.ErrDisconnected},
	}}
	replacement := &reconnectingManagedBarrierFixture{}
	reconnectCalls := 0
	connection, err := newReconnectingManagedSession(
		initial,
		registration,
		managedBarrierTestLaunchContext(),
		func(context.Context, managedsession.LaunchContext) (managedSessionClient, managedsession.RegisterResponse, error) {
			reconnectCalls++
			return replacement, registration, nil
		},
	)
	if err != nil {
		t.Fatalf("newReconnectingManagedSession() error = %v", err)
	}
	if err := waitForManagedComposeBarrier(t.Context(), connection, registration, "orders"); err != nil {
		t.Fatalf("waitForManagedComposeBarrier() error = %v", err)
	}
	if reconnectCalls != 1 || initial.closeCalls != 1 || len(initial.barrierRequests) != 1 || len(replacement.barrierRequests) != 1 {
		t.Fatalf("reconnect calls=%d initial closes=%d initial barriers=%d replacement barriers=%d", reconnectCalls, initial.closeCalls, len(initial.barrierRequests), len(replacement.barrierRequests))
	}
	if replacement.barrierRequests[0].Fence != registration.Fence {
		t.Fatalf("replayed barrier fence = %#v, want %#v", replacement.barrierRequests[0].Fence, registration.Fence)
	}
}

// TestReconnectingManagedSessionDoesNotReconnectRemotePolicyFailures keeps authorization and validation failures terminal.
func TestReconnectingManagedSessionDoesNotReconnectRemotePolicyFailures(t *testing.T) {
	initial := &reconnectingManagedBarrierFixture{recordingManagedBarrierClient: recordingManagedBarrierClient{
		replaceErrors: []error{managedsession.RemoteError{Failure: managedsession.WireError{Code: managedsession.ErrorCode("permission_denied"), Message: "denied"}}},
	}}
	connection, err := newReconnectingManagedSession(
		initial,
		managedBarrierTestRegistration(t),
		managedBarrierTestLaunchContext(),
		func(context.Context, managedsession.LaunchContext) (managedSessionClient, managedsession.RegisterResponse, error) {
			t.Fatal("reconnect callback must not run for a remote policy failure")
			return nil, managedsession.RegisterResponse{}, errors.New("unreachable")
		},
	)
	if err != nil {
		t.Fatalf("newReconnectingManagedSession() error = %v", err)
	}
	if err := waitForManagedComposeBarrier(t.Context(), connection, managedBarrierTestRegistration(t), "orders"); err == nil {
		t.Fatal("waitForManagedComposeBarrier() accepted a remote policy failure")
	}
	if initial.closeCalls != 0 {
		t.Fatalf("initial close calls = %d, want no reconnect cleanup", initial.closeCalls)
	}
}

// TestReconnectingManagedSessionRuntimePlanUsesNegotiatedCapability proves the assignment call keeps the current fence.
func TestReconnectingManagedSessionRuntimePlanUsesNegotiatedCapability(t *testing.T) {
	registration := managedBarrierTestRegistration(t)
	client := &reconnectingManagedBarrierFixture{recordingManagedBarrierClient: recordingManagedBarrierClient{
		peer: managedsession.Peer{Capabilities: []managedsession.Capability{managedsession.CapabilityRuntimePlanV1}},
	}}
	connection, err := newReconnectingManagedSession(
		client,
		registration,
		managedBarrierTestLaunchContext(),
		func(context.Context, managedsession.LaunchContext) (managedSessionClient, managedsession.RegisterResponse, error) {
			t.Fatal("runtime plan test must not reconnect")
			return nil, managedsession.RegisterResponse{}, errors.New("unreachable")
		},
	)
	if err != nil {
		t.Fatalf("newReconnectingManagedSession() error = %v", err)
	}
	if !connection.RuntimePlanAvailable() {
		t.Fatal("RuntimePlanAvailable() = false, want negotiated capability")
	}
	response, err := connection.RuntimePlan(t.Context(), managedsession.RuntimePlanRequest{
		SchemaVersion: managedsession.SchemaVersion,
		Fence:         registration.Fence,
		ActiveApps:    []managedsession.ActiveApp{},
	})
	if err != nil {
		t.Fatalf("RuntimePlan() error = %v", err)
	}
	if response.Fence != registration.Fence || len(client.runtimeRequests) != 1 || client.runtimeRequests[0].Fence != registration.Fence {
		t.Fatalf("runtime plan response/request = %#v / %#v, want registration fence %#v", response, client.runtimeRequests, registration.Fence)
	}

	client.peer.Capabilities = nil
	if connection.RuntimePlanAvailable() {
		t.Fatal("RuntimePlanAvailable() = true without negotiated capability")
	}
}

// TestWaitForManagedComposeBarrierRejectsPermanentErrors verifies a non-retryable remote failure returns immediately.
func TestWaitForManagedComposeBarrierRejectsPermanentErrors(t *testing.T) {
	registration := managedBarrierTestRegistration(t)
	client := &recordingManagedBarrierClient{
		replaceErrors: []error{
			managedsession.RemoteError{Failure: managedsession.WireError{Code: managedsession.ErrorCode("permission_denied"), Message: "denied"}},
		},
	}
	if err := waitForManagedComposeBarrier(t.Context(), client, registration, "orders"); err == nil {
		t.Fatal("waitForManagedComposeBarrier() accepted a permanent error")
	}
	if len(client.barrierRequests) != 0 {
		t.Fatalf("barrier requests = %#v, want none after permanent replacement failure", client.barrierRequests)
	}
}

// TestManagedProjectIdentityNormalizesPresentationNames verifies accepted identities are portable wire tokens.
func TestManagedProjectIdentityNormalizesPresentationNames(t *testing.T) {
	if got := managedProjectIdentity(&project.Config{ProjectName: "Diablo Immortal Tracker"}, "/private/tmp/diablo"); got != "diablo-immortal-tracker" {
		t.Fatalf("managedProjectIdentity() = %q, want normalized project identity", got)
	}
	if got := managedProjectIdentity(nil, "/private/tmp/orders"); got != "orders" {
		t.Fatalf("managedProjectIdentity() fallback = %q, want orders", got)
	}
	if got := managedProjectIdentity(&project.Config{ProjectName: "!!!"}, "/private/tmp/orders"); got != "project" {
		t.Fatalf("managedProjectIdentity() empty normalization = %q, want project", got)
	}
}
