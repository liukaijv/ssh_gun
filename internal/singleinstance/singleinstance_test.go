package singleinstance

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestActivateNotify(t *testing.T) {
	var got atomic.Bool
	server, err := startActivateServer(func() {
		got.Store(true)
	})
	if err != nil {
		t.Fatalf("startActivateServer: %v", err)
	}
	defer server.close()

	if err := notifyActivateImpl(); err != nil {
		t.Fatalf("NotifyActivate: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("onActivate was not called")
}

func TestAcquireSecondInstance(t *testing.T) {
	first, err := Acquire()
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Close()

	second, err := Acquire()
	if !errors.Is(err, ErrAlreadyRunning) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("second Acquire: got %v, want ErrAlreadyRunning", err)
	}
}

func TestListenActivateAndNotify(t *testing.T) {
	guard, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer guard.Close()

	var got atomic.Bool
	if err := guard.ListenActivate(func() { got.Store(true) }); err != nil {
		t.Fatalf("ListenActivate: %v", err)
	}

	if err := NotifyActivate(); err != nil {
		t.Fatalf("NotifyActivate: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("onActivate was not called via Guard")
}
