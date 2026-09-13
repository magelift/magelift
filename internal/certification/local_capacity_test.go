package certification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewMemoryLocalCapacityReservationRejectsInvalidCapacityAndRequests(t *testing.T) {
	tests := []struct {
		name     string
		capacity LocalCapacityRequest
		request  LocalCapacityRequest
	}{
		{name: "negative capacity CPU", capacity: LocalCapacityRequest{CPUMilli: -1}},
		{name: "negative capacity memory", capacity: LocalCapacityRequest{MemoryMB: -1}},
		{name: "negative request CPU", capacity: LocalCapacityRequest{CPUMilli: 100}, request: LocalCapacityRequest{CPUMilli: -1}},
		{name: "negative request memory", capacity: LocalCapacityRequest{MemoryMB: 100}, request: LocalCapacityRequest{MemoryMB: -1}},
		{name: "request exceeds capacity", capacity: LocalCapacityRequest{CPUMilli: 100}, request: LocalCapacityRequest{CPUMilli: 101}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reservation, err := NewMemoryLocalCapacityReservation(test.capacity)
			if test.capacity.CPUMilli < 0 || test.capacity.MemoryMB < 0 {
				if err == nil {
					t.Fatal("negative reservation capacity was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = reservation.Acquire(context.Background(), test.request)
			if err == nil {
				t.Fatal("negative reservation request was accepted")
			}
		})
	}
}

func TestMemoryLocalCapacityReservationCancellationDoesNotConsumeCapacity(t *testing.T) {
	request := LocalCapacityRequest{CPUMilli: 500, MemoryMB: 256}
	reservation, err := NewMemoryLocalCapacityReservation(request)
	if err != nil {
		t.Fatal(err)
	}
	holding, err := reservation.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, acquireErr := reservation.Acquire(ctx, request)
		result <- acquireErr
	}()
	cancel()

	select {
	case acquireErr := <-result:
		if !errors.Is(acquireErr, context.Canceled) {
			t.Fatalf("canceled acquire error = %v", acquireErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled acquire did not return")
	}
	if err := holding.Release(); err != nil {
		t.Fatal(err)
	}

	available, err := reservation.Acquire(context.Background(), request)
	if err != nil {
		t.Fatalf("capacity was stranded after cancellation: %v", err)
	}
	if err := available.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryLocalCapacityReservationReleaseIsIdempotentAndUnblocksFIFO(t *testing.T) {
	capacity := LocalCapacityRequest{CPUMilli: 1000, MemoryMB: 1024}
	reservation, err := NewMemoryLocalCapacityReservation(capacity)
	if err != nil {
		t.Fatal(err)
	}
	holding, err := reservation.Acquire(context.Background(), capacity)
	if err != nil {
		t.Fatal(err)
	}

	firstResult := make(chan LocalCapacityLease, 1)
	firstErr := make(chan error, 1)
	go func() {
		lease, acquireErr := reservation.Acquire(context.Background(), capacity)
		if acquireErr != nil {
			firstErr <- acquireErr
			return
		}
		firstResult <- lease
	}()

	select {
	case acquireErr := <-firstErr:
		t.Fatal(acquireErr)
	case <-firstResult:
		t.Fatal("capacity was acquired before release")
	case <-time.After(25 * time.Millisecond):
	}
	if err := holding.Release(); err != nil {
		t.Fatal(err)
	}
	if err := holding.Release(); err != nil {
		t.Fatal(err)
	}

	var waiting LocalCapacityLease
	select {
	case acquireErr := <-firstErr:
		t.Fatal(acquireErr)
	case waiting = <-firstResult:
	case <-time.After(2 * time.Second):
		t.Fatal("waiting acquire did not receive released capacity")
	}
	if err := waiting.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireLocalCapacityFailsClosedForMissingOrNilReservation(t *testing.T) {
	request := &LocalCapacityRequest{CPUMilli: 1}
	if _, err := acquireLocalCapacity(context.Background(), nil, request); err == nil || !strings.Contains(err.Error(), "reservation is required") {
		t.Fatalf("missing reservation error = %v", err)
	}
	var typedNil *MemoryLocalCapacityReservation
	if _, err := acquireLocalCapacity(context.Background(), typedNil, request); err == nil || !strings.Contains(err.Error(), "reservation is required") {
		t.Fatalf("typed nil reservation error = %v", err)
	}
	if _, err := acquireLocalCapacity(context.Background(), nil, &LocalCapacityRequest{CPUMilli: -1}); err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("invalid request error = %v", err)
	}
}
