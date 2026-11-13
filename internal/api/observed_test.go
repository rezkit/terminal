package api

import (
	"context"
	"testing"
	"time"
)

func TestObservedReportsRequests(t *testing.T) {
	o := Observe(NewFake())
	// Without hooks, calls pass straight through.
	ws, err := o.Workspaces(context.Background())
	if err != nil || len(ws) == 0 {
		t.Fatalf("Workspaces: %v, %v", ws, err)
	}

	var started, finished int
	o.SetHooks(Hooks{
		Started:  func() { started++ },
		Finished: func(d time.Duration) { finished++ },
	})
	o.SetWorkspace(ws[0]) // not a request
	if _, err := o.Bookings(context.Background(), BookingQuery{Page: PageRequest{Size: 5}}); err != nil {
		t.Fatal(err)
	}
	if started != 1 || finished != 1 {
		t.Fatalf("started=%d finished=%d, want 1/1", started, finished)
	}
}
