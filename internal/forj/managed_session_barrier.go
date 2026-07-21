package forj

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/goforj/goforj/internal/managedsession"
	"github.com/goforj/goforj/project"
)

const managedComposeBarrierRetryDelay = 100 * time.Millisecond

// managedBarrierClient is the narrow client surface needed to synchronize one managed Compose session.
type managedBarrierClient interface {
	ReplacePublications(context.Context, managedsession.ReplacePublicationsRequest) (managedsession.ReplacePublicationsResponse, error)
	Barrier(context.Context, managedsession.BarrierRequest) (managedsession.BarrierResponse, error)
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
