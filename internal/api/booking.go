package api

import (
	"slices"
	"strings"
	"time"
)

// BookingDetail is everything the booking screen shows.
//
// Transactions and Notes are nil when the API withheld them (no permission)
// and empty when there are none; see the *Denied helpers.
type BookingDetail struct {
	ID, Reference, Status, Description, Currency string
	Tags                                         []string
	Created, Updated                             time.Time
	Start, End                                   time.Time // zero when unknown
	Customer                                     *Customer
	Total, Paid, Outstanding                     Money
	Passengers                                   []Passenger
	Reservations                                 []Reservation // sorted by start date
	Transactions                                 []Transaction
	Notes                                        []Note
}

// TransactionsDenied reports whether payments were withheld by the API.
func (b BookingDetail) TransactionsDenied() bool { return b.Transactions == nil }

// NotesDenied reports whether notes were withheld by the API.
func (b BookingDetail) NotesDenied() bool { return b.Notes == nil }

// Passenger looks a passenger up by ID.
func (b BookingDetail) Passenger(id string) (Passenger, bool) {
	for _, p := range b.Passengers {
		if p.ID == id {
			return p, true
		}
	}
	return Passenger{}, false
}

type Customer struct {
	Honorific, Given, Family, Email, Telephone string
	Address                                    []string // formatted lines
}

func (c Customer) Name() string { return joinName(c.Honorific, c.Given, c.Family) }

type Passenger struct {
	ID, Honorific, Given, Family, Email string
	DateOfBirth, Cancelled              time.Time // zero when unknown / not cancelled
}

func (p Passenger) Name() string { return joinName(p.Honorific, p.Given, p.Family) }

// AgeAt returns the passenger's age in whole years at t, or -1 if unknown.
func (p Passenger) AgeAt(t time.Time) int {
	if p.DateOfBirth.IsZero() || t.IsZero() {
		return -1
	}
	dob, at := Civil(p.DateOfBirth), Civil(t)
	age := at.Year() - dob.Year()
	if at.Month() < dob.Month() || (at.Month() == dob.Month() && at.Day() < dob.Day()) {
		age--
	}
	return age
}

type Reservation struct {
	ID, Reference, Product, Category, Description string
	Status, StatusMessage                         string
	Quantity                                      int
	Start, End, BalanceDue                        time.Time // zero when unknown
	Price                                         *Money
	PassengerIDs                                  []string
	Legs                                          []Leg // transport slices, by departure; empty for other products
}

type Transaction struct {
	ID, Reference, Description, Category, Status string
	Created                                      time.Time
	Amount                                       *Money
}

type Note struct {
	ID, Title, Content string
	Created            time.Time
}

func joinName(parts ...string) string {
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// SortReservations orders reservations by start date (undated last), then
// reference, so list and itinerary views share one order.
func SortReservations(rs []Reservation) {
	slices.SortStableFunc(rs, func(a, b Reservation) int {
		switch {
		case a.Start.IsZero() != b.Start.IsZero():
			if a.Start.IsZero() {
				return 1
			}
			return -1
		case !a.Start.Equal(b.Start):
			return a.Start.Compare(b.Start)
		}
		return strings.Compare(a.Reference, b.Reference)
	})
}

// Civil returns t for calendar display. Date-only values arrive as midnight
// UTC and are kept in UTC (so they don't shift a day west of Greenwich);
// real instants are shown in local time.
func Civil(t time.Time) time.Time {
	if u := t.UTC(); u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 && u.Nanosecond() == 0 {
		return u
	}
	return t.Local()
}

// Leg is one transport slice (a flight, train, transfer...) of a transport
// reservation, with the passengers travelling on it.
type Leg struct {
	ID, Name, Mode       string // Mode: Air, Rail, Road, Water
	Carrier, CarrierCode string
	From, To             Stop
	Depart, Arrive       time.Time // in the respective stop's timezone when known
	IntermediateStop     bool
	Passengers           []LegPassenger
}

type Stop struct{ Code, Name, City, Timezone string }

// Label is the stop's code, falling back to its name.
func (s Stop) Label() string {
	if s.Code != "" {
		return s.Code
	}
	return s.Name
}

type LegPassenger struct{ PassengerID, Class, Ticket, Seat string }

// Number is the carrier code and service name, e.g. "BA 728".
func (l Leg) Number() string { return joinName(l.CarrierCode, l.Name) }

// InZone returns t in the IANA timezone tz, or unchanged if tz is unknown.
func InZone(t time.Time, tz string) time.Time {
	if tz == "" {
		return t
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return t
	}
	return t.In(loc)
}

// SortLegs orders legs by departure.
func SortLegs(ls []Leg) {
	slices.SortStableFunc(ls, func(a, b Leg) int { return a.Depart.Compare(b.Depart) })
}
