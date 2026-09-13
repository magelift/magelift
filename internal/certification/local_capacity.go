package certification

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// LocalCapacityRequest describes the runner capacity one schedule unit needs
// while its provider callback is active. CPU is expressed in milli-CPU and
// memory in MiB so callers do not have to depend on a provider's native
// resource type.
type LocalCapacityRequest struct {
	CPUMilli int64 `json:"cpuMilli" yaml:"cpuMilli"`
	MemoryMB int64 `json:"memoryMb" yaml:"memoryMb"`
}

func (request LocalCapacityRequest) Validate() error {
	if request.CPUMilli < 0 {
		return errors.New("local capacity CPU cannot be negative")
	}
	if request.MemoryMB < 0 {
		return errors.New("local capacity memory cannot be negative")
	}
	return nil
}

func (request LocalCapacityRequest) empty() bool {
	return request.CPUMilli == 0 && request.MemoryMB == 0
}

func (request LocalCapacityRequest) fits(available LocalCapacityRequest) bool {
	return request.CPUMilli <= available.CPUMilli && request.MemoryMB <= available.MemoryMB
}

func (request LocalCapacityRequest) subtract(available LocalCapacityRequest) LocalCapacityRequest {
	return LocalCapacityRequest{
		CPUMilli: available.CPUMilli - request.CPUMilli,
		MemoryMB: available.MemoryMB - request.MemoryMB,
	}
}

// LocalCapacityLease is an acquired local-capacity claim. Release is
// idempotent so every scheduler exit path can defer it without having to
// coordinate whether a provider callback or checkpoint write failed first.
type LocalCapacityLease interface {
	Release() error
}

// LocalCapacityReservation is the injectable local runner admission boundary.
// Implementations must atomically acquire both CPU and memory, wait for
// capacity with context cancellation, and make Release safe to call exactly
// once or more than once. A distributed runner can provide the same contract
// with a conditional shared-store implementation.
type LocalCapacityReservation interface {
	Acquire(context.Context, LocalCapacityRequest) (LocalCapacityLease, error)
}

// MemoryLocalCapacityReservation coordinates capacity within one process.
// It intentionally does not claim to coordinate another host; callers that
// run certification on multiple machines must inject a shared implementation.
type MemoryLocalCapacityReservation struct {
	mu        sync.Mutex
	capacity  LocalCapacityRequest
	available LocalCapacityRequest
	waiters   []*memoryLocalCapacityWaiter
}

type memoryLocalCapacityWaiter struct {
	request LocalCapacityRequest
	ready   chan struct{}
	queued  bool
	granted bool
}

// NewMemoryLocalCapacityReservation creates a process-local reservation with
// the supplied total capacity. A zero dimension is valid and means that no
// unit requiring that dimension can be admitted.
func NewMemoryLocalCapacityReservation(capacity LocalCapacityRequest) (*MemoryLocalCapacityReservation, error) {
	if err := capacity.Validate(); err != nil {
		return nil, fmt.Errorf("local capacity reservation capacity: %w", err)
	}
	return &MemoryLocalCapacityReservation{capacity: capacity, available: capacity}, nil
}

var _ LocalCapacityReservation = (*MemoryLocalCapacityReservation)(nil)

func (reservation *MemoryLocalCapacityReservation) Acquire(ctx context.Context, request LocalCapacityRequest) (LocalCapacityLease, error) {
	if reservation == nil {
		return nil, errors.New("local capacity reservation is required")
	}
	if ctx == nil {
		return nil, errors.New("local capacity reservation context is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.CPUMilli > reservation.capacity.CPUMilli || request.MemoryMB > reservation.capacity.MemoryMB {
		return nil, errors.New("local capacity request exceeds reservation capacity")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.empty() {
		return localCapacityNoopLease{}, nil
	}

	waiter := &memoryLocalCapacityWaiter{request: request, ready: make(chan struct{}), queued: true}
	reservation.mu.Lock()
	if len(reservation.waiters) == 0 && request.fits(reservation.available) {
		reservation.available = request.subtract(reservation.available)
		reservation.mu.Unlock()
		return &memoryLocalCapacityLease{reservation: reservation, request: request}, nil
	}
	reservation.waiters = append(reservation.waiters, waiter)
	reservation.grantWaitersLocked()
	reservation.mu.Unlock()

	select {
	case <-waiter.ready:
		return &memoryLocalCapacityLease{reservation: reservation, request: request}, nil
	case <-ctx.Done():
		reservation.mu.Lock()
		switch {
		case waiter.granted:
			// The ready and cancellation signals raced. Return the capacity
			// before reporting cancellation so no lease can be stranded.
			waiter.granted = false
			reservation.available.CPUMilli += request.CPUMilli
			reservation.available.MemoryMB += request.MemoryMB
			reservation.grantWaitersLocked()
		case waiter.queued:
			reservation.removeWaiterLocked(waiter)
			reservation.grantWaitersLocked()
		}
		reservation.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (reservation *MemoryLocalCapacityReservation) grantWaitersLocked() {
	for len(reservation.waiters) > 0 {
		waiter := reservation.waiters[0]
		if !waiter.request.fits(reservation.available) {
			return
		}
		reservation.available = waiter.request.subtract(reservation.available)
		reservation.waiters = reservation.waiters[1:]
		waiter.queued = false
		waiter.granted = true
		close(waiter.ready)
	}
}

func (reservation *MemoryLocalCapacityReservation) removeWaiterLocked(target *memoryLocalCapacityWaiter) {
	for index, waiter := range reservation.waiters {
		if waiter != target {
			continue
		}
		copy(reservation.waiters[index:], reservation.waiters[index+1:])
		reservation.waiters[len(reservation.waiters)-1] = nil
		reservation.waiters = reservation.waiters[:len(reservation.waiters)-1]
		target.queued = false
		return
	}
}

func (reservation *MemoryLocalCapacityReservation) release(request LocalCapacityRequest) {
	reservation.mu.Lock()
	reservation.available.CPUMilli += request.CPUMilli
	reservation.available.MemoryMB += request.MemoryMB
	reservation.grantWaitersLocked()
	reservation.mu.Unlock()
}

type memoryLocalCapacityLease struct {
	reservation *MemoryLocalCapacityReservation
	request     LocalCapacityRequest
	once        sync.Once
}

func (lease *memoryLocalCapacityLease) Release() error {
	if lease == nil {
		return errors.New("local capacity lease is required")
	}
	lease.once.Do(func() {
		lease.reservation.release(lease.request)
	})
	return nil
}

type localCapacityNoopLease struct{}

func (localCapacityNoopLease) Release() error { return nil }

func acquireLocalCapacity(ctx context.Context, reservation LocalCapacityReservation, request *LocalCapacityRequest) (LocalCapacityLease, error) {
	if request == nil {
		return nil, nil
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.empty() {
		return nil, nil
	}
	if ctx == nil {
		return nil, errors.New("local capacity reservation context is required")
	}
	if isNilLocalCapacityReservation(reservation) {
		return nil, errors.New("local capacity reservation is required")
	}
	lease, err := reservation.Acquire(ctx, *request)
	if err != nil {
		return nil, fmt.Errorf("acquire local capacity: %w", err)
	}
	if isNilLocalCapacityLease(lease) {
		return nil, errors.New("local capacity reservation returned a nil lease")
	}
	return lease, nil
}

func isNilLocalCapacityReservation(reservation LocalCapacityReservation) bool {
	if reservation == nil {
		return true
	}
	value := reflect.ValueOf(reservation)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func isNilLocalCapacityLease(lease LocalCapacityLease) bool {
	if lease == nil {
		return true
	}
	value := reflect.ValueOf(lease)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
