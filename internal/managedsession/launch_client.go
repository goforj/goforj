package managedsession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"
)

const inheritedLaunchRegistrationTimeout = 10 * time.Second

// OpenLaunchSession dials Harbor and retries only the bounded startup race before process evidence is durable.
func OpenLaunchSession(ctx context.Context, launch LaunchContext) (*Client, RegisterResponse, error) {
	if err := launch.Validate(); err != nil {
		return nil, RegisterResponse{}, fmt.Errorf("validate managed launch context: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	registrationContext, cancel := context.WithTimeout(ctx, inheritedLaunchRegistrationTimeout)
	defer cancel()
	client, err := Dial(registrationContext, func(dialContext context.Context) (net.Conn, error) {
		return dialManagedEndpoint(dialContext, launch.EndpointReference)
	}, ClientConfig{Capabilities: []Capability{CapabilityV1, CapabilityLaunchContextV1}})
	if err != nil {
		return nil, RegisterResponse{}, err
	}
	nonce, err := newManagedSessionNonce()
	if err != nil {
		_ = client.Close()
		return nil, RegisterResponse{}, err
	}
	peer := client.Peer()
	if !containsCapability(peer.Capabilities, CapabilityLaunchContextV1) {
		_ = client.Close()
		return nil, RegisterResponse{}, errors.New("managed session launch context capability was not negotiated")
	}
	response, err := client.RegisterUntilAttached(registrationContext, RegisterRequest{
		SchemaVersion:             SchemaVersion,
		ProjectID:                 launch.ProjectID,
		SessionID:                 launch.SessionID,
		ProjectRoot:               launch.ProjectRoot,
		ExpectedSessionGeneration: launch.ExpectedSessionGeneration,
		DescriptorDigest:          launch.DescriptorDigest,
		ClientNonce:               nonce,
		Owner:                     SessionOwnerHarbor,
		Capabilities:              append([]Capability(nil), peer.Capabilities...),
		ActiveApps:                []ActiveApp{},
		LaunchTicket:              launch.Ticket,
	})
	if err != nil {
		_ = client.Close()
		return nil, RegisterResponse{}, fmt.Errorf("register managed launch session: %w", err)
	}
	retainedLaunch := launch
	client.launchContext = &retainedLaunch
	return client, response, nil
}

// newManagedSessionNonce creates a short-lived request identity without using project configuration.
func newManagedSessionNonce() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate managed session client nonce: %w", err)
	}
	return hex.EncodeToString(value), nil
}
