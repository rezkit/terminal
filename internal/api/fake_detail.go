package api

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"strings"
	"time"
)

func (f *Fake) Booking(ctx context.Context, id string) (BookingDetail, error) {
	if err := f.wait(ctx); err != nil {
		return BookingDetail{}, err
	}
	all, err := f.current()
	if err != nil {
		return BookingDetail{}, err
	}
	for i, b := range all {
		if b.ID == id {
			return fakeDetail(b, i), nil
		}
	}
	return BookingDetail{}, fmt.Errorf("booking %s not found", id)
}

var (
	fakeGiven  = []string{"Ada", "William", "Byron", "Ralph", "Charles", "Mary", "Alan", "Grace", "Emmy", "Kurt"}
	fakeHotels = []string{"Hotel Monte Rosa", "Riffelalp Resort", "Grand Hotel Zermatterhof", "Fjordhotel Lofoten",
		"Desert Camp Merzouga", "Hotel Explora", "Ryokan Kurashiki", "Villa Cimbrone"}
	fakeActs = []string{"Guided glacier hike", "Fjord kayak tour", "Camel trek at sunset", "Torres del Paine day walk",
		"Tea ceremony", "Sailing to Positano", "Golden Circle tour", "Machu Picchu sunrise visit"}
	fakeGroups    = map[string]string{"Accommodation": "Partner hotels", "Activity": "Guided excursions"}
	fakeSuppliers = map[string]string{"Flight": "SkyLink Air", "Transfer": "Alpine Transfers", "Accommodation": "StayBridge Wholesale"}
	fakeNotes     = []Note{
		{Title: "Dietary requirements", Content: "Lead passenger is vegetarian; one child has a severe nut allergy and carries an EpiPen."},
		{Title: "Room preference", Content: "Quiet rooms away from the lift; interconnecting if possible."},
		{Title: "Accessibility", Content: "Ground-floor room requested; limited mobility on stairs."},
	}
)

// fakeDetail builds a plausible, deterministic itinerary for a list booking.
func fakeDetail(b Booking, idx int) BookingDetail {
	h := fnv.New64a()
	h.Write([]byte(b.ID))
	r := rand.New(rand.NewPCG(h.Sum64(), 7))

	start := time.Date(b.StartDate.Year(), b.StartDate.Month(), b.StartDate.Day(), 0, 0, 0, 0, time.UTC)
	nights := 4 + r.IntN(10)
	end := start.AddDate(0, 0, nights)
	given, family, _ := strings.Cut(b.Customer, " ")
	cur := b.Total.Currency

	d := BookingDetail{
		ID: b.ID, Reference: b.Reference, Status: b.Status, Description: b.Description, Currency: cur,
		Created: b.CreatedAt, Updated: b.CreatedAt.Add(time.Duration(1+r.IntN(500)) * time.Hour),
		Start: start, End: end, Total: b.Total,
		Customer: &Customer{
			Given: given, Family: family,
			Email:     strings.ToLower(given + "." + family + "@example.com"),
			Telephone: fmt.Sprintf("+44 20 7946 %04d", r.IntN(10000)),
			Address:   []string{fmt.Sprintf("%d Marylebone Road", 1+r.IntN(200)), "London NW1 5LR", "GB"},
		},
	}
	if r.IntN(3) == 0 {
		d.Tags = []string{"repeat-customer"}
	}

	// Passengers: the customer plus family members; sometimes one cancelled.
	pax := 1 + r.IntN(5)
	for i := range pax {
		p := Passenger{ID: fmt.Sprintf("%s-p%d", b.ID, i+1), Given: given, Family: family}
		if i > 0 {
			p.Given = fakeGiven[r.IntN(len(fakeGiven))]
		}
		p.DateOfBirth = time.Date(1950+r.IntN(70), time.Month(1+r.IntN(12)), 1+r.IntN(28), 0, 0, 0, 0, time.UTC)
		if i < 2 {
			p.Email = strings.ToLower(p.Given + "." + family + "@example.com")
		}
		if i == pax-1 && pax > 2 && r.IntN(3) == 0 {
			p.Cancelled = b.CreatedAt.AddDate(0, 0, 20)
		}
		d.Passengers = append(d.Passengers, p)
	}
	var travelling []string
	for _, p := range d.Passengers {
		if p.Cancelled.IsZero() {
			travelling = append(travelling, p.ID)
		}
	}

	// Reservations: flights, transfer, one or two stays, an activity.
	split := nights / 2
	if nights < 6 {
		split = nights
	}
	type item struct {
		product, category, element string
		from, to                   int // day offsets
		weight                     int
	}
	items := []item{
		{"Outbound flight", "Flight", "Economy Light", 0, 0, 12},
		{"Airport transfer", "Transfer", "Private car (up to 4)", 0, 0, 3},
		{fakeHotels[r.IntN(len(fakeHotels))], "Accommodation", "Double room, half board", 0, split, 30},
		{fakeActs[r.IntN(len(fakeActs))], "Activity", "", 1 + r.IntN(max(nights-1, 1)), -1, 8},
	}
	if split < nights {
		items = append(items, item{fakeHotels[r.IntN(len(fakeHotels))], "Accommodation", "Junior suite, B&B", split, nights, 30})
	}
	items = append(items, item{"Return flight", "Flight", "Economy Light", nights, nights, 12})

	dest := fakeAirports[r.IntN(len(fakeAirports))]
	totalW := 0
	for _, it := range items {
		totalW += it.weight
	}
	remaining := b.Total.Minor
	for i, it := range items {
		price := b.Total.Minor * int64(it.weight) / int64(totalW)
		if i == len(items)-1 {
			price = remaining
		}
		remaining -= price
		res := Reservation{
			ID: fmt.Sprintf("%s-r%d", b.ID, i+1), Reference: fmt.Sprintf("PR-%05d", idx*10+i+1),
			Product: it.product, Category: it.category, Element: it.element, Status: b.Status, Quantity: 1,
			Group: fakeGroups[it.category], Supplier: fakeSuppliers[it.category],
			Start: start.AddDate(0, 0, it.from), Price: &Money{price, cur},
			BalanceDue:   start.AddDate(0, 0, -42),
			PassengerIDs: travelling,
		}
		if it.to >= 0 {
			res.End = start.AddDate(0, 0, it.to)
		}
		if it.category != "Accommodation" {
			res.Quantity = len(travelling)
		}
		if b.Status == "CONFIRMED" && i == 3 && r.IntN(2) == 0 {
			res.Status, res.StatusMessage = "ON_HOLD", "Awaiting supplier confirmation"
		}
		switch it.category { // descriptions are optional free text
		case "Transfer":
			res.Description = "Driver meets you in arrivals with a name board"
		case "Activity":
			res.Description = "Meet in the hotel lobby at 08:30; bring walking boots"
		}
		res.Legs = fakeLegs(r, it.category, i, res.Start, dest, travelling)
		d.Reservations = append(d.Reservations, res)
	}
	SortReservations(d.Reservations)

	// Payments: a deposit, maybe an instalment, then a pending balance.
	deposit := b.Total.Minor / 5
	d.Transactions = []Transaction{{
		ID: b.ID + "-t1", Reference: fmt.Sprintf("TX-%05d", idx*10+1), Description: "Deposit",
		Category: "CARD", Status: "CONFIRMED", Created: b.CreatedAt.Add(10 * time.Minute), Amount: &Money{deposit, cur},
	}}
	paid := deposit
	if r.IntN(2) == 0 {
		inst := b.Total.Minor / 4
		d.Transactions = append(d.Transactions, Transaction{
			ID: b.ID + "-t2", Reference: fmt.Sprintf("TX-%05d", idx*10+2), Description: "Instalment",
			Category: "BANK_TRANSFER", Status: "CONFIRMED", Created: b.CreatedAt.AddDate(0, 1, 0), Amount: &Money{inst, cur},
		})
		paid += inst
	}
	d.Transactions = append(d.Transactions, Transaction{
		ID: b.ID + "-t3", Reference: fmt.Sprintf("TX-%05d", idx*10+3), Description: "Balance",
		Category: "CARD", Status: "PENDING", Created: d.Updated, Amount: &Money{b.Total.Minor - paid, cur},
	})
	d.Paid = Money{paid, cur}
	d.Outstanding = Money{b.Total.Minor - paid, cur}

	// Notes: none, some, or withheld (to show the access-denied state).
	switch {
	case idx%11 == 5:
		d.Notes = nil
	default:
		d.Notes = []Note{}
		for i := range r.IntN(3) {
			n := fakeNotes[(idx+i)%len(fakeNotes)]
			n.ID, n.Created = fmt.Sprintf("%s-n%d", b.ID, i+1), b.CreatedAt.AddDate(0, 0, i)
			d.Notes = append(d.Notes, n)
		}
	}
	return d
}

type fakeAirport struct {
	stop  Stop
	hours int // flying time from London
}

var (
	fakeHome     = Stop{Code: "LHR", Name: "London Heathrow", City: "London", Timezone: "Europe/London"}
	fakeHub      = Stop{Code: "AMS", Name: "Amsterdam Schiphol", City: "Amsterdam", Timezone: "Europe/Amsterdam"}
	fakeAirports = []fakeAirport{
		{Stop{"GVA", "Geneva", "Geneva", "Europe/Zurich"}, 2},
		{Stop{"BGO", "Bergen Flesland", "Bergen", "Europe/Oslo"}, 2},
		{Stop{"RAK", "Marrakesh Menara", "Marrakesh", "Africa/Casablanca"}, 4},
		{Stop{"NAP", "Naples", "Naples", "Europe/Rome"}, 3},
		{Stop{"KEF", "Keflavík", "Reykjavík", "Atlantic/Reykjavik"}, 3},
		{Stop{"KIX", "Kansai", "Osaka", "Asia/Tokyo"}, 14},
		{Stop{"LIM", "Jorge Chávez", "Lima", "America/Lima"}, 13},
	}
)

// fakeLegs gives flights (sometimes via a hub) and transfers their slices.
func fakeLegs(r *rand.Rand, category string, item int, day time.Time, dest fakeAirport, pax []string) []Leg {
	at := func(s Stop, d time.Time, h, m int) time.Time {
		loc, err := time.LoadLocation(s.Timezone)
		if err != nil {
			loc = time.UTC
		}
		return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc)
	}
	leg := func(n int, carrier, code string, from, to Stop, dep time.Time, dur time.Duration, mode string) Leg {
		l := Leg{ID: fmt.Sprintf("leg-%d-%d", item, n), Mode: mode,
			Carrier: carrier, CarrierCode: code, From: from, To: to,
			Depart: dep, Arrive: InZone(dep.Add(dur), to.Timezone)}
		if mode == "Air" {
			l.Name = fmt.Sprint(100 + r.IntN(900))
		}
		row := 10 + r.IntN(30)
		for i, id := range pax {
			lp := LegPassenger{PassengerID: id}
			if mode == "Air" {
				lp.Class, lp.Seat = "Economy", fmt.Sprintf("%d%c", row, 'A'+i)
				lp.Ticket = fmt.Sprintf("125-%010d", r.Int64N(1e10))
			}
			l.Passengers = append(l.Passengers, lp)
		}
		return l
	}
	flight := time.Duration(dest.hours) * time.Hour
	switch {
	case category == "Transfer":
		hotel := Stop{Name: "Hotel", City: dest.stop.City, Timezone: dest.stop.Timezone}
		dep := at(dest.stop, day, 15+r.IntN(4), 0)
		return []Leg{leg(1, "Alpine Transfers", "", dest.stop, hotel, dep, 90*time.Minute, "Road")}
	case category != "Flight":
		return nil
	case item == 0 && r.IntN(2) == 0: // outbound via the hub
		dep := at(fakeHome, day, 6+r.IntN(3), 5*r.IntN(12))
		first := leg(1, "KLM", "KL", fakeHome, fakeHub, dep, 75*time.Minute, "Air")
		second := leg(2, "KLM", "KL", fakeHub, dest.stop, first.Arrive.Add(95*time.Minute), flight-30*time.Minute, "Air")
		return []Leg{first, second}
	case item == 0:
		return []Leg{leg(1, "British Airways", "BA", fakeHome, dest.stop,
			at(fakeHome, day, 7+r.IntN(4), 5*r.IntN(12)), flight, "Air")}
	}
	return []Leg{leg(1, "British Airways", "BA", dest.stop, fakeHome,
		at(dest.stop, day, 10+r.IntN(10), 5*r.IntN(12)), flight, "Air")}
}
