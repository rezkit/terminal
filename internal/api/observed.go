package api

import (
	"context"
	"sync/atomic"
	"time"
)

// Hooks are told when a Service request starts and finishes. Either may be
// nil. They run on the goroutine making the request.
type Hooks struct {
	Started  func()
	Finished func(elapsed time.Duration)
}

// Observed wraps a Service and reports every request to its hooks, so the UI
// can show activity and timings for any implementation (including Fake).
type Observed struct {
	svc   Service
	hooks atomic.Pointer[Hooks]
}

var _ Service = (*Observed)(nil)

// Observe wraps svc. Hooks can be set later with SetHooks.
func Observe(svc Service) *Observed { return &Observed{svc: svc} }

// SetHooks replaces the hooks; safe to call while requests are running.
func (o *Observed) SetHooks(h Hooks) { o.hooks.Store(&h) }

// track reports the start of a request and returns a func reporting its end.
func (o *Observed) track() func() {
	h := o.hooks.Load()
	if h == nil {
		return func() {}
	}
	if h.Started != nil {
		h.Started()
	}
	start := time.Now()
	return func() {
		if h.Finished != nil {
			h.Finished(time.Since(start))
		}
	}
}

func (o *Observed) Workspaces(ctx context.Context) ([]Workspace, error) {
	defer o.track()()
	return o.svc.Workspaces(ctx)
}

func (o *Observed) SetWorkspace(w Workspace) { o.svc.SetWorkspace(w) }

func (o *Observed) Bookings(ctx context.Context, q BookingQuery) (Page[Booking], error) {
	defer o.track()()
	return o.svc.Bookings(ctx, q)
}

func (o *Observed) Booking(ctx context.Context, id string) (BookingDetail, error) {
	defer o.track()()
	return o.svc.Booking(ctx, id)
}

func (o *Observed) Customers(ctx context.Context, q CustomerQuery) (Page[CustomerRow], error) {
	defer o.track()()
	return o.svc.Customers(ctx, q)
}

func (o *Observed) Customer(ctx context.Context, id string) (CustomerDetail, error) {
	defer o.track()()
	return o.svc.Customer(ctx, id)
}

func (o *Observed) Transactions(ctx context.Context, q TransactionQuery) (Page[TransactionRow], error) {
	defer o.track()()
	return o.svc.Transactions(ctx, q)
}
