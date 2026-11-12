package api

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fake is an in-memory Service used by tests and demo mode. Data is
// deterministic per workspace.
type Fake struct {
	Latency time.Duration

	mu         sync.Mutex
	workspaces []Workspace
	workspace  string
	bookings   map[string][]Booking       // by workspace ID
	details    map[string][]BookingDetail // derived lazily from bookings; read-only
}

// NewFake returns a Fake with two workspaces of generated bookings.
func NewFake() *Fake {
	f := &Fake{
		workspaces: []Workspace{
			{ID: "ws-alpine", Name: "Alpine Adventures"},
			{ID: "ws-coastal", Name: "Coastal Cruises"},
		},
		bookings: map[string][]Booking{},
		details:  map[string][]BookingDetail{},
	}
	for i, ws := range f.workspaces {
		f.bookings[ws.ID] = generateBookings(uint64(i+1), 237-i*100)
	}
	return f
}

var (
	fakeCustomers = []string{"Ada Lovelace", "Alan Turing", "Grace Hopper", "Edsger Dijkstra",
		"Barbara Liskov", "Donald Knuth", "Margaret Hamilton", "Ken Thompson", "Radia Perlman", "John McCarthy"}
	fakeProducts = []string{"Matterhorn Trek", "Fjord Explorer", "Sahara Nights", "Patagonia Circuit",
		"Kyoto Temples", "Amalfi Coast Sail", "Iceland Ring Road", "Inca Trail"}
)

func generateBookings(seed uint64, n int) []Booking {
	r := rand.New(rand.NewPCG(seed, 42))
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]Booking, n)
	for i := range out {
		created := base.Add(time.Duration(r.IntN(300*24)) * time.Hour)
		out[i] = Booking{
			ID:          fmt.Sprintf("bk_%04d", i+1),
			Reference:   fmt.Sprintf("RZ-%05d", 10000+i*7),
			Status:      BookingStatuses[r.IntN(len(BookingStatuses))],
			Customer:    fakeCustomers[r.IntN(len(fakeCustomers))],
			Description: fakeProducts[r.IntN(len(fakeProducts))] + " group",
			StartDate:   created.Add(time.Duration(14+r.IntN(200)) * 24 * time.Hour),
			Total:       Money{Minor: int64(50000 + r.IntN(900000)), Currency: "GBP"},
			CreatedAt:   created,
		}
	}
	return out
}

func (f *Fake) wait(ctx context.Context) error {
	if f.Latency == 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(f.Latency):
		return nil
	}
}

func (f *Fake) Workspaces(ctx context.Context) ([]Workspace, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return slices.Clone(f.workspaces), nil
}

func (f *Fake) SetWorkspace(w Workspace) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workspace = w.ID
}

func (f *Fake) current() ([]Booking, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.bookings[f.workspace]
	if !ok {
		return nil, fmt.Errorf("no workspace selected")
	}
	return b, nil
}

func (f *Fake) Bookings(ctx context.Context, q BookingQuery) (Page[Booking], error) {
	if err := f.wait(ctx); err != nil {
		return Page[Booking]{}, err
	}
	all, err := f.current()
	if err != nil {
		return Page[Booking]{}, err
	}
	var rows []Booking
	search := strings.ToLower(q.Filter.Reference)
	for _, b := range all {
		if search != "" && !strings.Contains(strings.ToLower(b.Reference), search) {
			continue
		}
		if q.Filter.Status != "" && b.Status != q.Filter.Status {
			continue
		}
		if !q.Filter.CreatedFrom.IsZero() && b.CreatedAt.Before(q.Filter.CreatedFrom) {
			continue
		}
		if !q.Filter.CreatedTo.IsZero() && b.CreatedAt.After(q.Filter.CreatedTo) {
			continue
		}
		rows = append(rows, b)
	}
	slices.SortStableFunc(rows, func(a, b Booking) int {
		c := compareBooking(a, b, q.Sort.Field)
		if q.Sort.Desc {
			return -c
		}
		return c
	})
	return paginate(rows, q.Page)
}

func compareBooking(a, b Booking, field string) int {
	switch field {
	case "reference":
		return strings.Compare(a.Reference, b.Reference)
	case "description":
		return strings.Compare(a.Description, b.Description)
	default: // createdAt
		return a.CreatedAt.Compare(b.CreatedAt)
	}
}

func paginate[T any](rows []T, p PageRequest) (Page[T], error) {
	size := p.Size
	if size <= 0 {
		size = 25
	}
	offset := 0
	if p.Cursor != "" {
		n, err := strconv.Atoi(p.Cursor)
		if err != nil || n < 0 {
			return Page[T]{}, fmt.Errorf("invalid cursor %q", p.Cursor)
		}
		offset = n
	}
	offset = min(offset, len(rows))
	end := min(offset+size, len(rows))
	info := PageInfo{Total: len(rows)}
	if end < len(rows) {
		info.NextCursor = strconv.Itoa(end)
	}
	return Page[T]{Items: rows[offset:end], Info: info}, nil
}
