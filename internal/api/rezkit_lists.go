package api

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rezkit/terminal/internal/api/graphql"
	"github.com/rezkit/terminal/internal/api/scalar"
)

// Helpers shared by the RezKit list and detail queries.

// contains is a case-insensitive substring filter.
func contains(s string) *graphql.StringFilter {
	op := graphql.StringFilterOperationContains
	return &graphql.StringFilter{Operation: &op, Value: &s}
}

// timeRange is an inclusive timestamp filter; nil when both ends are open.
func timeRange(from, to time.Time) *graphql.TimestampFilter {
	if from.IsZero() && to.IsZero() {
		return nil
	}
	tf := &graphql.TimestampFilter{}
	if !from.IsZero() {
		tf.Since = &scalar.Timestamp{Time: from}
	}
	if !to.IsZero() {
		tf.Until = &scalar.Timestamp{Time: to}
	}
	return tf
}

// direction maps a Sort onto the API's direction argument.
func direction(s Sort) *graphql.SortDirection {
	d := graphql.SortDirectionAsc
	if s.Desc {
		d = graphql.SortDirectionDesc
	}
	return &d
}

// pageArgs maps a PageRequest onto first/after.
func pageArgs(p PageRequest) (*int, *string) {
	first := p.Size
	if p.Cursor == "" {
		return &first, nil
	}
	after := p.Cursor
	return &first, &after
}

// pageInfo maps a Relay connection's paging fields.
func pageInfo(total *int, hasNext bool, endCursor *string) PageInfo {
	info := PageInfo{Total: -1}
	if total != nil {
		info.Total = *total
	}
	if hasNext && endCursor != nil {
		info.NextCursor = *endCursor
	}
	return info
}

func money(amount int, currency string) Money { return Money{int64(amount), currency} }

// --- customers --------------------------------------------------------------------

var customerSorts = map[string]graphql.CustomerSort{
	SortFamilyName: graphql.CustomerSortFamilyName,
	SortEmail:      graphql.CustomerSortEmail,
	SortCity:       graphql.CustomerSortAddressCity,
	SortCreatedAt:  graphql.CustomerSortCreatedAt,
}

// customerFilters maps the UI filter onto the API's; nil when unfiltered.
func customerFilters(f CustomerFilter) *graphql.CustomerFilters {
	var out graphql.CustomerFilters
	used := false
	set := func(dst **graphql.StringFilter, v string) {
		if v != "" {
			*dst, used = contains(v), true
		}
	}
	set(&out.GivenName, f.Given)
	set(&out.FamilyName, f.Family)
	set(&out.Email, f.Email)
	if tf := timeRange(f.CreatedFrom, f.CreatedTo); tf != nil {
		out.CreatedAt, used = tf, true
	}
	if !used {
		return nil
	}
	return &out
}

func (r *RezKit) Customers(ctx context.Context, q CustomerQuery) (Page[CustomerRow], error) {
	org, err := r.workspace()
	if err != nil {
		return Page[CustomerRow]{}, err
	}
	first, after := pageArgs(q.Page)
	var sortBy *graphql.CustomerSort
	var dir *graphql.SortDirection
	if s, ok := customerSorts[q.Sort.Field]; ok {
		sortBy, dir = &s, direction(q.Sort)
	}
	resp, err := graphql.ListCustomers(ctx, r.client, org, customerFilters(q.Filter), first, after, sortBy, dir)
	if err != nil {
		return Page[CustomerRow]{}, err
	}
	conn := resp.Customers.List
	page := Page[CustomerRow]{Info: PageInfo{Total: -1}}
	if pi := conn.PageInfo; pi != nil {
		page.Info = pageInfo(conn.TotalCount, pi.HasNextPage, pi.EndCursor)
	} else {
		page.Info = pageInfo(conn.TotalCount, false, nil)
	}
	for _, e := range conn.Edges {
		if e == nil || e.Node == nil {
			continue
		}
		n := e.Node
		c := CustomerRow{
			ID: n.Id, Honorific: deref(n.Honorifics), Given: n.GivenName, Family: n.FamilyName,
			Email: n.Email, Telephone: deref(n.Telephone), Created: n.CreatedAt.Time,
		}
		if a := n.Address; a != nil {
			c.City, c.Country = a.City, a.Country
		}
		page.Items = append(page.Items, c)
	}
	return page, nil
}

func (r *RezKit) Customer(ctx context.Context, id string) (CustomerDetail, error) {
	resp, err := graphql.GetCustomer(ctx, r.client, id)
	if err != nil {
		return CustomerDetail{}, err
	}
	c := resp.Customers.Customer
	if c == nil {
		return CustomerDetail{}, fmt.Errorf("customer %s not found (or %w)", id, ErrAccessDenied)
	}
	d := CustomerDetail{
		ID: c.Id,
		Customer: Customer{
			Honorific: deref(c.Honorifics), Given: c.GivenName, Family: c.FamilyName,
			Email: c.Email, Telephone: deref(c.Telephone),
		},
		KnownAs: deref(c.KnownAs), DateOfBirth: tsPtr(c.DateOfBirth),
		Created: c.CreatedAt.Time, Updated: c.UpdatedAt.Time, Tags: c.Tags,
	}
	if c.Sex != nil {
		d.Sex = string(*c.Sex)
	}
	if a := c.Address; a != nil {
		d.Address = addressLines(a.Line1, deref(a.Line2), deref(a.Line3),
			strings.TrimSpace(a.City+" "+a.PostalCode), deref(a.State), a.Country)
	}
	if c.Bookings != nil { // nil = withheld; keep it nil
		d.Bookings = []CustomerBooking{}
		for _, b := range c.Bookings {
			d.Bookings = append(d.Bookings, CustomerBooking{
				ID: b.Id, Reference: b.Reference, Status: string(b.Status), Description: deref(b.Description),
				Created: b.CreatedAt.Time, Start: tsPtr(b.Dates.Start), End: tsPtr(b.Dates.End),
				Total:       money(b.TotalPrice.Amount, b.TotalPrice.Currency),
				Outstanding: money(b.OutstandingBalance.Amount, b.OutstandingBalance.Currency),
			})
		}
		SortCustomerBookings(d.Bookings)
	}
	if c.Notes != nil {
		d.Notes = []Note{}
		for _, n := range c.Notes {
			d.Notes = append(d.Notes, Note{ID: n.Id, Title: n.Title, Content: n.Content, Created: n.CreatedAt.Time})
		}
	}
	return d, nil
}

// SortCustomerBookings orders bookings newest first by start date (undated
// ones by creation), so upcoming and recent trips lead.
func SortCustomerBookings(bs []CustomerBooking) {
	when := func(b CustomerBooking) time.Time {
		if b.Start.IsZero() {
			return b.Created
		}
		return b.Start
	}
	slices.SortStableFunc(bs, func(a, b CustomerBooking) int {
		return cmp.Or(when(b).Compare(when(a)), strings.Compare(a.Reference, b.Reference))
	})
}

// --- transactions -----------------------------------------------------------------

func transactionFilters(f TransactionFilter) *graphql.TransactionFilters {
	var out graphql.TransactionFilters
	used := false
	if f.Reference != "" {
		out.Reference, used = contains(f.Reference), true
	}
	if f.BookingRef != "" {
		out.Booking, used = &graphql.BookingFilters{Reference: contains(f.BookingRef)}, true
	}
	if f.Currency != "" {
		out.AmountCurrency = &graphql.StringChoiceFilter{
			Operation: graphql.StringChoiceFilterOperationInclude, Values: []string{strings.ToUpper(f.Currency)}}
		used = true
	}
	if tf := timeRange(f.CreatedFrom, f.CreatedTo); tf != nil {
		out.CreatedAt, used = tf, true
	}
	if !used {
		return nil
	}
	return &out
}

func (r *RezKit) Transactions(ctx context.Context, q TransactionQuery) (Page[TransactionRow], error) {
	org, err := r.workspace()
	if err != nil {
		return Page[TransactionRow]{}, err
	}
	first, after := pageArgs(q.Page)
	resp, err := graphql.ListTransactions(ctx, r.client, org, transactionFilters(q.Filter), first, after)
	if err != nil {
		return Page[TransactionRow]{}, err
	}
	conn := resp.Transactions
	page := Page[TransactionRow]{}
	if pi := conn.PageInfo; pi != nil {
		page.Info = pageInfo(conn.TotalCount, pi.HasNextPage, pi.EndCursor)
	} else {
		page.Info = pageInfo(conn.TotalCount, false, nil)
	}
	for _, e := range conn.Edges {
		if e == nil || e.Node == nil {
			continue
		}
		n := e.Node
		t := TransactionRow{
			Transaction: Transaction{
				ID: n.Id, Reference: deref(n.Reference), Description: deref(n.Description),
				Category: string(n.Category), Status: string(n.Status), Created: n.CreatedAt.Time,
			},
			BookingID: n.Booking.Id, BookingRef: n.Booking.Reference,
		}
		if a := n.Amount; a != nil {
			m := money(a.Amount, a.Currency)
			t.Amount = &m
		}
		if c := n.Booking.Customer; c != nil {
			t.Customer = joinName(c.GivenName, c.FamilyName)
		}
		page.Items = append(page.Items, t)
	}
	return page, nil
}
