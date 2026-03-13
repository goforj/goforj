package queue

import "testing"

func TestNewManagerBuildsDefaultQueue(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if got := mgr.Default().Driver(); got != "null" {
		t.Fatalf("Default driver = %q, want %q", got, "null")
	}
}

func TestNewManagerSupportsNamedQueues(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if got := mgr.Critical().Driver(); got != "sync" {
		t.Fatalf("Critical driver = %q, want %q", got, "sync")
	}
}
