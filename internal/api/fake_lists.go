package api

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	fakeFamilies = []string{"Babbage", "Noether", "Gödel", "Shannon", "Johnson", "Bartik", "Wilkes", "Ritchie"}
	fakeCities   = []struct{ city, country string }{
		{"London", "GB"}, {"Manchester", "GB"}, {"Edinburgh", "GB"}, {"Dublin", "IE"},
		{"Amsterdam", "NL"}, {"Berlin", "DE"}, {"Lyon", "FR"}, {"Bristol", "GB"},
	}
)

// fakeCustomerRows builds a workspace's customers: everyone named on a
// booking, then extra customers who have never booked.
func fakeCustomerRows(bookings []Booking) []CustomerRow {
	first := map[string]time.Time{}
	for _, b := range bookings {
		if t, ok := first[b.Customer]; !ok || b.CreatedAt.Before(t) {
			first[b.Customer] = b.CreatedAt
		}
	}
	var names []string
	for _, n := range fakeCustomers {
		if _, ok := first[n]; ok {
			names = append(names, n)
		}
	}
	for i, g := range fakeGiven {
		for j, f := range fakeFamilies {
			if (i+j)%2 == 0 {
				names = append(names, g+" "+f)
			}
		}
	}
	base := time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC)
	out := make([]CustomerRow, len(names))
	for i, n := range names {
		given, family, _ := strings.Cut(n, " ")
		loc := fakeCities[i%len(fakeCities)]
		c := CustomerRow{
			ID: fmt.Sprintf("cu_%03d", i+1), Given: given, Family: family,
			Email:     strings.ToLower(given + "." + family + "@example.com"),
			Telephone: fmt.Sprintf("+44 20 7946 %04d", (i*7919)%10000),
			City:      loc.city, Country: loc.country,
			Created: base.Add(time.Duration(i*53) * time.Hour),
		}
		if t, ok := first[n]; ok {
			c.Created = t.AddDate(0, 0, -3)
		}
		if i%9 == 4 {
			c.Telephone = ""
		}
		out[i] = c
	}
	return out
}

func (f *Fake) customers() ([]CustomerRow, []Booking, error) {
	bookings, err := f.current()
	if err != nil {
		return nil, nil, err
	}
	return fakeCustomerRows(bookings), bookings, nil
}

func (f *Fake) Customers(ctx context.Context, q CustomerQuery) (Page[CustomerRow], error) {
	if err := f.wait(ctx); err != nil {
		return Page[CustomerRow]{}, err
	}
	all, _, err := f.customers()
	if err != nil {
		return Page[CustomerRow]{}, err
	}
	has := func(s, sub string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(sub)) }
	var rows []CustomerRow
	for _, c := range all {
		switch {
		case !has(c.Given, q.Filter.Given), !has(c.Family, q.Filter.Family), !has(c.Email, q.Filter.Email),
			!q.Filter.CreatedFrom.IsZero() && c.Created.Before(q.Filter.CreatedFrom),
			!q.Filter.CreatedTo.IsZero() && c.Created.After(q.Filter.CreatedTo):
			continue
		}
		rows = append(rows, c)
	}
	slices.SortStableFunc(rows, func(a, b CustomerRow) int {
		var c int
		switch q.Sort.Field {
		case SortFamilyName:
			c = strings.Compare(a.Family+" "+a.Given, b.Family+" "+b.Given)
		case SortEmail:
			c = strings.Compare(a.Email, b.Email)
		case SortCity:
			c = strings.Compare(a.City, b.City)
		default:
			c = a.Created.Compare(b.Created)
		}
		if q.Sort.Desc {
			return -c
		}
		return c
	})
	return paginate(rows, q.Page)
}

func (f *Fake) Customer(ctx context.Context, id string) (CustomerDetail, error) {
	if err := f.wait(ctx); err != nil {
		return CustomerDetail{}, err
	}
	all, _, err := f.customers()
	if err != nil {
		return CustomerDetail{}, err
	}
	i := slices.IndexFunc(all, func(c CustomerRow) bool { return c.ID == id })
	if i < 0 {
		return CustomerDetail{}, fmt.Errorf("customer %s not found", id)
	}
	c := all[i]
	d := CustomerDetail{
		ID: c.ID,
		Customer: Customer{Given: c.Given, Family: c.Family, Email: c.Email, Telephone: c.Telephone,
			Address: []string{fmt.Sprintf("%d High Street", 1+i*13%180), c.City, c.Country}},
		DateOfBirth: time.Date(1955+i*7%50, time.Month(1+i%12), 1+i*5%28, 0, 0, 0, 0, time.UTC),
		Created:     c.Created, Updated: c.Created.AddDate(0, 2, 0),
		Bookings: []CustomerBooking{},
	}
	if i%3 == 0 {
		d.Honorific = "Dr"
	}
	if i%4 == 1 {
		d.KnownAs = c.Given[:min(3, len(c.Given))]
	}
	if i%5 == 0 {
		d.Tags = []string{"vip", "newsletter"}
	}
	bookings, details, err := f.allDetails()
	if err != nil {
		return CustomerDetail{}, err
	}
	name := c.Given + " " + c.Family
	for bi, b := range bookings {
		if b.Customer != name {
			continue
		}
		det := details[bi]
		d.Bookings = append(d.Bookings, CustomerBooking{
			ID: b.ID, Reference: b.Reference, Status: b.Status, Description: b.Description,
			Created: b.CreatedAt, Start: det.Start, End: det.End, Total: b.Total, Outstanding: det.Outstanding,
		})
	}
	SortCustomerBookings(d.Bookings)
	if i == 7 { // bookings withheld for one customer, to show the access-denied state
		d.Bookings = nil
	}
	switch { // notes: withheld for one customer, to show the access-denied state
	case i == 4:
	default:
		d.Notes = []Note{}
		for n := range i % 3 {
			note := fakeNotes[(i+n)%len(fakeNotes)]
			note.ID, note.Created = fmt.Sprintf("%s-n%d", c.ID, n+1), c.Created.AddDate(0, n, 1)
			d.Notes = append(d.Notes, note)
		}
	}
	return d, nil
}

func (f *Fake) Transactions(ctx context.Context, q TransactionQuery) (Page[TransactionRow], error) {
	if err := f.wait(ctx); err != nil {
		return Page[TransactionRow]{}, err
	}
	bookings, details, err := f.allDetails()
	if err != nil {
		return Page[TransactionRow]{}, err
	}
	has := func(s, sub string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(sub)) }
	fl := q.Filter
	var rows []TransactionRow
	for i, b := range bookings {
		if !has(b.Reference, fl.BookingRef) {
			continue
		}
		for _, t := range details[i].Transactions {
			switch {
			case !has(t.Reference, fl.Reference),
				fl.Currency != "" && (t.Amount == nil || !strings.EqualFold(t.Amount.Currency, fl.Currency)),
				!fl.CreatedFrom.IsZero() && t.Created.Before(fl.CreatedFrom),
				!fl.CreatedTo.IsZero() && t.Created.After(fl.CreatedTo):
				continue
			}
			rows = append(rows, TransactionRow{Transaction: t, BookingID: b.ID, BookingRef: b.Reference, Customer: b.Customer})
		}
	}
	// The API takes no sort for transactions; newest first is the fake's order.
	slices.SortStableFunc(rows, func(a, b TransactionRow) int { return b.Created.Compare(a.Created) })
	return paginate(rows, q.Page)
}

// allDetails returns every booking's detail in the current workspace, in
// booking order, built once (the bookings never change). Callers must not
// modify the result.
func (f *Fake) allDetails() ([]Booking, []BookingDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bookings, ok := f.bookings[f.workspace]
	if !ok {
		return nil, nil, fmt.Errorf("no workspace selected")
	}
	ds, ok := f.details[f.workspace]
	if !ok {
		ds = make([]BookingDetail, len(bookings))
		for i, b := range bookings {
			ds[i] = fakeDetail(b, i)
		}
		f.details[f.workspace] = ds
	}
	return bookings, ds, nil
}
