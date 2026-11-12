package api

import "time"

// CustomerRow is a row in the customers list.
type CustomerRow struct {
	ID, Honorific, Given, Family, Email, Telephone string
	City, Country                                  string
	Created                                        time.Time
}

func (c CustomerRow) Name() string { return joinName(c.Honorific, c.Given, c.Family) }

// Location is "City, CC" from whatever parts are known.
func (c CustomerRow) Location() string {
	switch {
	case c.City == "":
		return c.Country
	case c.Country == "":
		return c.City
	}
	return c.City + ", " + c.Country
}

// CustomerFilter narrows the customers list. Zero values mean "any"; set
// fields are combined (AND) by the API.
type CustomerFilter struct {
	Given, Family, Email string // contains
	CreatedFrom          time.Time
	CreatedTo            time.Time
}

// CustomerQuery is a full customers list request.
type CustomerQuery struct {
	Filter CustomerFilter
	Sort   Sort
	Page   PageRequest
}

// Sort keys for customers, as supported server-side by the API.
const (
	SortFamilyName = "familyName"
	SortEmail      = "email"
	SortCity       = "city"
	// SortCreatedAt is shared with bookings.
)

// CustomerDetail is everything the customer screen shows.
//
// Bookings and Notes are nil when the API withheld them (no permission) and
// empty when there are none; see the *Denied helpers.
type CustomerDetail struct {
	ID string
	Customer
	KnownAs, Sex     string
	DateOfBirth      time.Time // zero when unknown
	Created, Updated time.Time
	Tags             []string
	Bookings         []CustomerBooking // newest first
	Notes            []Note
}

func (c CustomerDetail) BookingsDenied() bool { return c.Bookings == nil }
func (c CustomerDetail) NotesDenied() bool    { return c.Notes == nil }

// CustomerBooking summarises one of a customer's bookings.
type CustomerBooking struct {
	ID, Reference, Status, Description string
	Created, Start, End                time.Time // Start/End zero when unknown
	Total, Outstanding                 Money
}
