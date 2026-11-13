package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	gql "github.com/Khan/genqlient/graphql"

	"github.com/rezkit/terminal/internal/api/graphql"
	"github.com/rezkit/terminal/internal/api/scalar"
)

// ErrAccessDenied is returned when the API answers with null data, which is
// how RezKit reports fields the caller may not read (the reason only appears
// as a warning in the response extensions).
var ErrAccessDenied = errors.New("access denied by the RezKit API (try logging in again)")

// RezKit implements Service against the RezKit GraphQL API.
type RezKit struct {
	client gql.Client

	mu  sync.Mutex
	org string
}

// NewRezKit creates a client for endpoint using httpClient (which must add
// authentication; see NewHTTPClient).
func NewRezKit(endpoint string, httpClient *http.Client) *RezKit {
	return &RezKit{client: gql.NewClient(endpoint, httpClient)}
}

func (r *RezKit) SetWorkspace(w Workspace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.org = w.ID
}

func (r *RezKit) workspace() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.org == "" {
		return "", errors.New("no workspace selected")
	}
	return r.org, nil
}

func (r *RezKit) Workspaces(ctx context.Context) ([]Workspace, error) {
	resp, err := graphql.Organizations(ctx, r.client)
	if err != nil {
		return nil, err
	}
	if resp.Organizations == nil {
		return nil, ErrAccessDenied
	}
	out := make([]Workspace, 0, len(resp.Organizations))
	for _, o := range resp.Organizations {
		out = append(out, Workspace{ID: o.Id, Name: o.Name, Environment: string(o.Environment)})
	}
	return out, nil
}

var bookingSorts = map[string]graphql.BookingListSort{
	SortReference:   graphql.BookingListSortReference,
	SortDescription: graphql.BookingListSortDescription,
	SortCreatedAt:   graphql.BookingListSortCreatedAt,
	SortUpdatedAt:   graphql.BookingListSortUpdatedAt,
}

// bookingFilters maps the UI filter onto the API's filter input; nil when
// nothing is filtered.
func bookingFilters(f BookingFilter) *graphql.BookingFilters {
	var out graphql.BookingFilters
	used := false
	if f.Reference != "" {
		out.Reference = contains(f.Reference)
		used = true
	}
	if f.Status != "" {
		out.Status = &graphql.BookingStatusFilter{Include: []graphql.BookingStatus{graphql.BookingStatus(f.Status)}}
		used = true
	}
	if tf := timeRange(f.CreatedFrom, f.CreatedTo); tf != nil {
		out.CreatedAt = tf
		used = true
	}
	if !used {
		return nil
	}
	return &out
}

func (r *RezKit) Bookings(ctx context.Context, q BookingQuery) (Page[Booking], error) {
	org, err := r.workspace()
	if err != nil {
		return Page[Booking]{}, err
	}
	first, after := pageArgs(q.Page)
	var sortBy *graphql.BookingListSort
	var dir *graphql.SortDirection
	if s, ok := bookingSorts[q.Sort.Field]; ok {
		sortBy, dir = &s, direction(q.Sort)
	}

	resp, err := graphql.ListBookings(ctx, r.client, org, bookingFilters(q.Filter), first, after, sortBy, dir)
	if err != nil {
		return Page[Booking]{}, err
	}
	conn := resp.Bookings.List
	page := Page[Booking]{Info: PageInfo{Total: -1}}
	if conn.TotalCount != nil {
		page.Info.Total = *conn.TotalCount
	}
	if pi := conn.PageInfo; pi != nil && pi.HasNextPage && pi.EndCursor != nil {
		page.Info.NextCursor = *pi.EndCursor
	}
	for _, e := range conn.Edges {
		if e == nil || e.Node == nil {
			continue
		}
		n := e.Node
		b := Booking{
			ID:          n.Id,
			Reference:   n.Reference,
			Status:      string(n.Status),
			Description: deref(n.Description),
			CreatedAt:   n.CreatedAt.Time,
			Total:       Money{Minor: int64(n.TotalPrice.Amount), Currency: n.TotalPrice.Currency},
		}
		if n.Customer != nil {
			b.Customer = strings.TrimSpace(n.Customer.GivenName + " " + n.Customer.FamilyName)
		}
		if n.Dates.Start != nil {
			b.StartDate = n.Dates.Start.Time
		}
		page.Items = append(page.Items, b)
	}
	return page, nil
}

func (r *RezKit) Booking(ctx context.Context, id string) (BookingDetail, error) {
	resp, err := graphql.GetBooking(ctx, r.client, id)
	if err != nil {
		return BookingDetail{}, err
	}
	b := resp.Bookings.Booking
	if b == nil {
		return BookingDetail{}, fmt.Errorf("booking %s not found (or %w)", id, ErrAccessDenied)
	}

	d := BookingDetail{
		ID: b.Id, Reference: b.Reference, Status: string(b.Status),
		Description: deref(b.Description), Currency: b.Currency, Tags: b.Tags,
		Created: b.CreatedAt.Time, Updated: b.UpdatedAt.Time,
		Start: tsPtr(b.Dates.Start), End: tsPtr(b.Dates.End),
		Total:       Money{int64(b.TotalPrice.Amount), b.TotalPrice.Currency},
		Paid:        Money{int64(b.TotalPaid.Amount), b.TotalPaid.Currency},
		Outstanding: Money{int64(b.OutstandingBalance.Amount), b.OutstandingBalance.Currency},
	}
	if c := b.Customer; c != nil {
		d.Customer = &Customer{
			Honorific: deref(c.Honorifics), Given: c.GivenName, Family: c.FamilyName,
			Email: c.Email, Telephone: deref(c.Telephone),
		}
		if a := c.Address; a != nil {
			d.Customer.Address = addressLines(a.Line1, deref(a.Line2), deref(a.Line3),
				strings.TrimSpace(a.City+" "+a.PostalCode), deref(a.State), a.Country)
		}
	}
	for _, p := range b.Passengers {
		d.Passengers = append(d.Passengers, Passenger{
			ID: p.Id, Honorific: deref(p.Honorifics), Given: p.GivenName, Family: p.FamilyName,
			Email: deref(p.Email), DateOfBirth: tsPtr(p.DateOfBirth), Cancelled: tsPtr(p.CancelledAt),
		})
	}
	for _, pr := range b.ProductReservations {
		if pr == nil {
			continue
		}
		res := Reservation{
			ID: pr.GetId(), Reference: deref(pr.GetReference()), Description: deref(pr.GetDescription()),
			Status: string(pr.GetStatus()), StatusMessage: deref(pr.GetStatusMessage()),
			Quantity: pr.GetQuantity(),
			Start:    tsPtr(pr.GetServiceStart()), End: tsPtr(pr.GetServiceEnd()), BalanceDue: tsPtr(pr.GetBalanceDue()),
		}
		if p := pr.GetProduct(); p != nil {
			res.Product, res.Category = p.GetName(), p.GetCategory()
			res.Element, res.Group = deref(p.GetElement()), deref(p.GetGroupName())
			if pv := p.GetProvider(); pv != nil {
				res.Supplier = pv.Name
			}
		}
		if p := pr.GetPrice(); p != nil {
			res.Price = &Money{int64(p.Amount), p.Currency}
		}
		for _, p := range pr.GetPassengers() {
			res.PassengerIDs = append(res.PassengerIDs, p.Id)
		}
		if tr, ok := pr.(*graphql.GetBookingBookingsBookingQueryBookingProductReservationsTransportReservation); ok {
			res.Legs = transportLegs(tr)
		}
		d.Reservations = append(d.Reservations, res)
	}
	SortReservations(d.Reservations)

	if b.Transactions != nil { // nil = withheld; keep it nil
		d.Transactions = []Transaction{}
		for _, t := range b.Transactions {
			tx := Transaction{
				ID: t.Id, Reference: deref(t.Reference), Description: deref(t.Description),
				Category: string(t.Category), Status: string(t.Status), Created: t.CreatedAt.Time,
			}
			if a := t.Amount; a != nil {
				tx.Amount = &Money{int64(a.Amount), a.Currency}
			}
			d.Transactions = append(d.Transactions, tx)
		}
	}
	if b.Notes != nil {
		d.Notes = []Note{}
		for _, n := range b.Notes {
			d.Notes = append(d.Notes, Note{ID: n.Id, Title: n.Title, Content: n.Content, Created: n.CreatedAt.Time})
		}
	}
	return d, nil
}

func addressLines(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func tsPtr(t *scalar.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

// transportLegs groups the API's per-passenger slice rows into legs.
func transportLegs(tr *graphql.GetBookingBookingsBookingQueryBookingProductReservationsTransportReservation) []Leg {
	var legs []Leg
	index := map[string]int{}
	for _, sp := range tr.Slices {
		s := sp.Slice
		i, ok := index[s.Id]
		if !ok {
			from, to := sliceStop(s.Departure.SliceEnd), sliceStop(s.Arrival.SliceEnd)
			legs = append(legs, Leg{
				ID: s.Id, Name: deref(s.Name), Mode: string(s.Mode),
				Carrier: s.Carrier.Name, CarrierCode: deref(s.Carrier.Code),
				From: from, To: to,
				Depart:           InZone(s.Departure.Timestamp.Time, from.Timezone),
				Arrive:           InZone(s.Arrival.Timestamp.Time, to.Timezone),
				IntermediateStop: s.IntermediateStop,
			})
			i = len(legs) - 1
			index[s.Id] = i
		}
		legs[i].Passengers = append(legs[i].Passengers, LegPassenger{
			PassengerID: sp.Passenger.Id, Class: deref(sp.TicketClass),
			Ticket: deref(sp.TicketNumber), Seat: deref(sp.SeatAllocation),
		})
	}
	SortLegs(legs)
	return legs
}

func sliceStop(e graphql.SliceEnd) Stop {
	return Stop{Code: deref(e.Stop.Code), Name: e.Stop.Name, City: deref(e.Stop.City), Timezone: e.Stop.Timezone}
}
