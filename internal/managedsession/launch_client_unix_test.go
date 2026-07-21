//go:build !windows

package managedsession

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestOpenLaunchSessionOverUnixEndpoint verifies inherited context registration through the macOS/Unix transport seam.
func TestOpenLaunchSessionOverUnixEndpoint(t *testing.T) {
	endpoint := t.TempDir() + "/harbord.sock"
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		t.Fatalf("listen managed endpoint: %v", err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		serverDone <- runTestManagedSessionPeerWithCapabilities(connection, []Capability{CapabilityLaunchContextV1, CapabilityV1}, func(reader *frameReader, writer *frameWriter, protocol Version) error {
			message, err := readTestEnvelope(reader)
			if err != nil {
				return err
			}
			request, err := DecodeRegisterRequest(message.Payload)
			if err != nil {
				return err
			}
			if request.LaunchTicket != strings.Repeat("b", 64) || request.ProjectRoot == "" {
				return os.ErrInvalid
			}
			return writeTestResponse(writer, protocol, message.RequestID, RegisterResponse{
				SchemaVersion:    SchemaVersion,
				Fence:            ManagedPublicationFence{ProjectID: request.ProjectID, SessionID: request.SessionID, SessionGeneration: request.ExpectedSessionGeneration + 1},
				AttachmentTicket: "ticket-1",
			})
		})
	}()

	launch := validLaunchContext(t)
	launch.EndpointReference = endpoint
	client, response, err := OpenLaunchSession(context.Background(), launch)
	if err != nil {
		t.Fatalf("OpenLaunchSession() error = %v", err)
	}
	if response.Fence.ProjectID != launch.ProjectID || response.Fence.SessionID != launch.SessionID {
		t.Fatalf("launch response fence = %#v, want launch identity", response.Fence)
	}
	_ = client.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("managed endpoint server error = %v", err)
	}
}

// TestReconnectLaunchSessionReplaysRetainedIdentity verifies a fresh IPC connection can reuse the durable fence
// without exposing the inherited launch context to the caller or changing its expected generation.
func TestReconnectLaunchSessionReplaysRetainedIdentity(t *testing.T) {
	endpoint := t.TempDir() + "/harbord.sock"
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		t.Fatalf("listen managed endpoint: %v", err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	var registrationCount atomic.Int32
	go func() {
		var wait sync.WaitGroup
		wait.Add(2)
		errorsCh := make(chan error, 2)
		for range 2 {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				errorsCh <- acceptErr
				wait.Done()
				continue
			}
			go func(connection net.Conn) {
				defer wait.Done()
				errorsCh <- runTestManagedSessionPeerWithCapabilities(connection, []Capability{CapabilityLaunchContextV1, CapabilityV1}, func(reader *frameReader, writer *frameWriter, protocol Version) error {
					message, err := readTestEnvelope(reader)
					if err != nil {
						return err
					}
					request, err := DecodeRegisterRequest(message.Payload)
					if err != nil {
						return err
					}
					if request.LaunchTicket != strings.Repeat("b", 64) || request.ExpectedSessionGeneration != 1 {
						return os.ErrInvalid
					}
					ticket := "ticket-initial"
					if registrationCount.Add(1) > 1 {
						ticket = "ticket-replayed"
					}
					return writeTestResponse(writer, protocol, message.RequestID, RegisterResponse{
						SchemaVersion:    SchemaVersion,
						Fence:            ManagedPublicationFence{ProjectID: request.ProjectID, SessionID: request.SessionID, SessionGeneration: request.ExpectedSessionGeneration + 1},
						AttachmentTicket: ticket,
					})
				})
			}(connection)
		}
		wait.Wait()
		close(errorsCh)
		for serverErr := range errorsCh {
			if serverErr != nil {
				serverDone <- serverErr
				return
			}
		}
		serverDone <- nil
	}()

	launch := validLaunchContext(t)
	launch.EndpointReference = endpoint
	client, response, err := OpenLaunchSession(context.Background(), launch)
	if err != nil {
		t.Fatalf("OpenLaunchSession() error = %v", err)
	}
	reconnected, replayed, err := client.ReconnectLaunchSession(context.Background())
	if err != nil {
		_ = client.Close()
		t.Fatalf("ReconnectLaunchSession() error = %v", err)
	}
	if replayed.Fence != response.Fence || replayed.AttachmentTicket == response.AttachmentTicket {
		t.Fatalf("replayed response = %#v, initial = %#v; want same fence and fresh ticket", replayed, response)
	}
	_ = client.Close()
	_ = reconnected.Close()
	if err := <-serverDone; err != nil {
		t.Fatalf("managed endpoint server error = %v", err)
	}
}
