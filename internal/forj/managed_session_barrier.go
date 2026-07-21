package forj

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/goforj/goforj/internal/managedsession"
	"github.com/goforj/goforj/project"
)

const managedComposeBarrierRetryDelay = 100 * time.Millisecond

const managedComposeHeartbeatInterval = 5 * time.Second

// managedBarrierClient is the narrow client surface needed to synchronize one managed Compose session.
type managedBarrierClient interface {
	ReplacePublications(context.Context, managedsession.ReplacePublicationsRequest) (managedsession.ReplacePublicationsResponse, error)
	Barrier(context.Context, managedsession.BarrierRequest) (managedsession.BarrierResponse, error)
}

// managedRuntimePlanClient is the optional semantic assignment surface negotiated by newer Harbor daemons.
type managedRuntimePlanClient interface {
	RuntimePlan(context.Context, managedsession.RuntimePlanRequest) (managedsession.RuntimePlanResponse, error)
}

// managedSessionClient extends the barrier surface with the close operation needed when a broken client is replaced.
type managedSessionClient interface {
	managedBarrierClient
	Close() error
}

// managedSessionReconnect opens a fresh client for one inherited launch context.
type managedSessionReconnect func(context.Context, managedsession.LaunchContext) (managedSessionClient, managedsession.RegisterResponse, error)

// reconnectingManagedSession preserves one exact fence while replacing a lost local IPC connection.
type reconnectingManagedSession struct {
	mu           sync.Mutex
	client       managedSessionClient
	registration managedsession.RegisterResponse
	launch       managedsession.LaunchContext
	reconnect    managedSessionReconnect
}

// newReconnectingManagedSession validates the initial registration before enabling bounded transport recovery.
func newReconnectingManagedSession(
	client managedSessionClient,
	registration managedsession.RegisterResponse,
	launch managedsession.LaunchContext,
	reconnect managedSessionReconnect,
) (*reconnectingManagedSession, error) {
	if client == nil {
		return nil, errors.New("managed session client is required")
	}
	if err := registration.Validate(); err != nil {
		return nil, fmt.Errorf("validate managed session registration: %w", err)
	}
	if err := launch.Validate(); err != nil {
		return nil, fmt.Errorf("validate managed launch context: %w", err)
	}
	if reconnect == nil {
		return nil, errors.New("managed session reconnect function is required")
	}
	return &reconnectingManagedSession{
		client:       client,
		registration: registration,
		launch:       launch,
		reconnect:    reconnect,
	}, nil
}

// Close closes the current managed-session connection without attempting another replay.
func (session *reconnectingManagedSession) Close() error {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	client := session.client
	session.client = nil
	session.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Close()
}

// ReplacePublications forwards one complete observation and reconnects once when the IPC stream is lost.
func (session *reconnectingManagedSession) ReplacePublications(
	ctx context.Context,
	request managedsession.ReplacePublicationsRequest,
) (managedsession.ReplacePublicationsResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		client, registration := session.snapshot()
		if client == nil {
			return managedsession.ReplacePublicationsResponse{}, managedsession.ErrClosed
		}
		request.Fence = registration.Fence
		response, err := client.ReplacePublications(ctx, request)
		if err == nil {
			return response, nil
		}
		if !managedSessionTransportFailure(err) || attempt != 0 {
			return managedsession.ReplacePublicationsResponse{}, err
		}
		if err := session.reconnectAfterFailure(ctx, client); err != nil {
			return managedsession.ReplacePublicationsResponse{}, err
		}
	}
	return managedsession.ReplacePublicationsResponse{}, errors.New("managed session replacement retry exhausted")
}

// Barrier forwards one lifecycle barrier and reconnects once when the IPC stream is lost.
func (session *reconnectingManagedSession) Barrier(
	ctx context.Context,
	request managedsession.BarrierRequest,
) (managedsession.BarrierResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		client, registration := session.snapshot()
		if client == nil {
			return managedsession.BarrierResponse{}, managedsession.ErrClosed
		}
		request.Fence = registration.Fence
		response, err := client.Barrier(ctx, request)
		if err == nil {
			return response, nil
		}
		if !managedSessionTransportFailure(err) || attempt != 0 {
			return managedsession.BarrierResponse{}, err
		}
		if err := session.reconnectAfterFailure(ctx, client); err != nil {
			return managedsession.BarrierResponse{}, err
		}
	}
	return managedsession.BarrierResponse{}, errors.New("managed session barrier retry exhausted")
}

// RuntimePlan forwards one exact assignment request and reconnects once when the IPC stream is lost.
func (session *reconnectingManagedSession) RuntimePlan(
	ctx context.Context,
	request managedsession.RuntimePlanRequest,
) (managedsession.RuntimePlanResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		client, registration := session.snapshot()
		if client == nil {
			return managedsession.RuntimePlanResponse{}, managedsession.ErrClosed
		}
		planClient, supported := client.(managedRuntimePlanClient)
		if !supported {
			return managedsession.RuntimePlanResponse{}, errors.New("managed session runtime-plan capability is unavailable")
		}
		request.Fence = registration.Fence
		response, err := planClient.RuntimePlan(ctx, request)
		if err == nil {
			return response, nil
		}
		if !managedSessionTransportFailure(err) || attempt != 0 {
			return managedsession.RuntimePlanResponse{}, err
		}
		if err := session.reconnectAfterFailure(ctx, client); err != nil {
			return managedsession.RuntimePlanResponse{}, err
		}
	}
	return managedsession.RuntimePlanResponse{}, errors.New("managed session runtime-plan retry exhausted")
}

// RuntimePlanAvailable reports whether the current negotiated client advertises semantic assignments.
func (session *reconnectingManagedSession) RuntimePlanAvailable() bool {
	if session == nil {
		return false
	}
	client, _ := session.snapshot()
	peerClient, ok := client.(interface{ Peer() managedsession.Peer })
	if !ok {
		return false
	}
	for _, capability := range peerClient.Peer().Capabilities {
		if capability == managedsession.CapabilityRuntimePlanV1 {
			return true
		}
	}
	return false
}

// snapshot returns the current client and fence without holding the lock across transport I/O.
func (session *reconnectingManagedSession) snapshot() (managedSessionClient, managedsession.RegisterResponse) {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.client, session.registration
}

// reconnectAfterFailure swaps one failed connection only if no concurrent caller has already replaced it.
func (session *reconnectingManagedSession) reconnectAfterFailure(ctx context.Context, failed managedSessionClient) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.client == nil {
		return managedsession.ErrClosed
	}
	if session.client != failed {
		return nil
	}
	replacement, registration, err := session.reconnect(ctx, session.launch)
	if err != nil {
		return fmt.Errorf("reconnect managed session: %w", err)
	}
	if replacement == nil {
		return errors.New("managed session reconnect returned a nil client")
	}
	if err := registration.Validate(); err != nil {
		_ = replacement.Close()
		return fmt.Errorf("validate replayed managed session registration: %w", err)
	}
	old := session.client
	session.client = replacement
	session.registration = registration
	_ = old.Close()
	return nil
}

// managedSessionTransportFailure keeps reconnection limited to a lost local stream, never a remote policy result.
func managedSessionTransportFailure(err error) bool {
	return errors.Is(err, managedsession.ErrDisconnected) || errors.Is(err, managedsession.ErrClosed)
}

// runManagedSessionHeartbeat periodically resets and republishes the managed observation until the dev context ends.
func runManagedSessionHeartbeat(
	ctx context.Context,
	client *reconnectingManagedSession,
	registration managedsession.RegisterResponse,
	identity string,
	errWriter io.Writer,
) {
	ticker := time.NewTicker(managedComposeHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := waitForManagedComposeBarrier(ctx, client, registration, identity); err != nil && !errors.Is(err, context.Canceled) {
				_, _ = fmt.Fprintf(errWriter, "forj dev: managed session heartbeat unavailable: %v\n", err)
			}
		}
	}
}

// waitForManagedComposeBarrier publishes an empty client snapshot, then waits until Harbor activates its own observations.
//
// GoForj does not invent host ports. Harbor re-observes the supervised Compose project during the barrier call, so the
// empty replacement is only a complete reset of any stale client-side facts. The retry loop keeps normal development
// watchers alive while Harbor waits for process attachment, readiness, Docker observations, or native route evidence.
func waitForManagedComposeBarrier(
	ctx context.Context,
	client managedBarrierClient,
	registration managedsession.RegisterResponse,
	identity string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return errors.New("managed Compose barrier client is required")
	}
	if err := registration.Validate(); err != nil {
		return fmt.Errorf("validate managed Compose barrier registration: %w", err)
	}
	identity = strings.TrimSpace(identity)
	request := managedsession.ReplacePublicationsRequest{
		SchemaVersion: managedsession.SchemaVersion,
		Fence:         registration.Fence,
		Publications:  []managedsession.ManagedEndpointPublication{},
	}
	barrier := managedsession.BarrierRequest{
		SchemaVersion:           managedsession.SchemaVersion,
		Fence:                   registration.Fence,
		Phase:                   managedsession.BarrierPhaseCompose,
		AcceptedProjectIdentity: identity,
	}
	if err := barrier.Validate(); err != nil {
		return fmt.Errorf("validate managed Compose barrier: %w", err)
	}

	publicationsSent := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !publicationsSent {
			if _, err := client.ReplacePublications(ctx, request); err != nil {
				if !managedSessionRetryable(err) {
					return fmt.Errorf("replace managed publications: %w", err)
				}
				if err := waitForManagedBarrierRetry(ctx); err != nil {
					return err
				}
				continue
			}
			publicationsSent = true
		}

		response, err := client.Barrier(ctx, barrier)
		if err != nil {
			if !managedSessionRetryable(err) {
				return fmt.Errorf("acknowledge managed Compose barrier: %w", err)
			}
		} else if response.Acknowledged {
			return nil
		}
		if err := waitForManagedBarrierRetry(ctx); err != nil {
			return err
		}
	}
}

// managedSessionRetryable keeps retries limited to Harbor's explicit temporary-unavailable category.
func managedSessionRetryable(err error) bool {
	var remote managedsession.RemoteError
	return errors.As(err, &remote) && remote.Failure.Code == managedsession.ErrorCodeUnavailable && remote.Failure.Retryable
}

// waitForManagedBarrierRetry keeps a temporary startup race from creating a tight IPC loop.
func waitForManagedBarrierRetry(ctx context.Context) error {
	timer := time.NewTimer(managedComposeBarrierRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// managedProjectIdentity derives the portable accepted identity token Harbor records for a managed Compose session.
func managedProjectIdentity(config *project.Config, projectRoot string) string {
	raw := ""
	if config != nil {
		raw = strings.TrimSpace(config.ProjectName)
	}
	if raw == "" {
		raw = filepath.Base(projectRoot)
	}
	var builder strings.Builder
	separator := false
	for _, character := range strings.ToLower(raw) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			builder.WriteRune(character)
			separator = false
			continue
		}
		if character <= unicode.MaxASCII {
			separator = true
		}
	}
	identity := strings.Trim(builder.String(), "-")
	if identity == "" {
		return "project"
	}
	return identity
}
