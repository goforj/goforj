package forj

import (
	"context"
	"testing"

	"github.com/goforj/goforj/internal/managedsession"
	"github.com/goforj/goforj/project"
)

// recordingManagedBarrierClient records the complete replacement and barrier requests used by the retry helper.
type recordingManagedBarrierClient struct {
	replaceRequests []managedsession.ReplacePublicationsRequest
	barrierRequests []managedsession.BarrierRequest
	barrierReplies  []managedsession.BarrierResponse
	replaceErrors   []error
	barrierErrors   []error
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
