package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeGraphQL records the last request and answers with a canned response
// keyed by operation name.
type fakeGraphQL struct {
	t         *testing.T
	responses map[string]string
	lastOp    string
	lastVars  map[string]any
	lastAuth  string
}

func (f *fakeGraphQL) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Fatal(err)
	}
	f.lastOp, f.lastVars, f.lastAuth = req.OperationName, req.Variables, r.Header.Get("Authorization")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(f.responses[req.OperationName]))
}

func newTestRezKit(t *testing.T, responses map[string]string) (*RezKit, *fakeGraphQL) {
	f := &fakeGraphQL{t: t, responses: responses}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok"})
	return NewRezKit(srv.URL, NewHTTPClient(ts)), f
}

func TestRezKitWorkspaces(t *testing.T) {
	r, f := newTestRezKit(t, map[string]string{"Organizations": `{"data":{"organizations":[
		{"id":"org-1","name":"Alpine","environment":"Production"},
		{"id":"org-2","name":"Alpine Test","environment":"Sandbox"}]}}`})
	ws, err := r.Workspaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[1] != (Workspace{ID: "org-2", Name: "Alpine Test", Environment: "Sandbox"}) {
		t.Errorf("workspaces = %+v", ws)
	}
	if f.lastAuth != "Bearer tok" {
		t.Errorf("auth header = %q", f.lastAuth)
	}
}

func TestRezKitWorkspacesNullIsAccessDenied(t *testing.T) {
	// What the real API returns without valid credentials.
	r, _ := newTestRezKit(t, map[string]string{"Organizations": `{"data":{"organizations":null},
		"extensions":{"warnings":[{"message":"Access denied to this field."}]}}`})
	if _, err := r.Workspaces(context.Background()); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("err = %v, want ErrAccessDenied", err)
	}
}

func TestRezKitBookingsVariablesAndMapping(t *testing.T) {
	r, f := newTestRezKit(t, map[string]string{"ListBookings": `{"data":{"bookings":{"list":{
		"totalCount": 120,
		"pageInfo": {"hasNextPage": true, "endCursor": "c-2"},
		"edges": [{"node": {
			"id": "bk-1", "reference": "RZ1", "status": "ON_HOLD", "description": "Trip",
			"createdAt": "2026-03-04T05:06:07Z",
			"customer": {"givenName": "Ada", "familyName": "Lovelace"},
			"totalPrice": {"amount": 12345, "currency": "GBP"},
			"dates": {"start": "2026-06-01T00:00:00Z"}
		}}, null]
	}}}}`})

	if _, err := r.Bookings(context.Background(), BookingQuery{}); err == nil {
		t.Fatal("expected an error before a workspace is selected")
	}
	r.SetWorkspace(Workspace{ID: "org-1"})

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	page, err := r.Bookings(context.Background(), BookingQuery{
		Filter: BookingFilter{Reference: "RZ", Status: "CONFIRMED", CreatedFrom: from},
		Sort:   Sort{Field: SortReference, Desc: true},
		Page:   PageRequest{Size: 30, Cursor: "c-1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	v := f.lastVars
	if v["organizationId"] != "org-1" || v["first"] != float64(30) || v["after"] != "c-1" ||
		v["sortBy"] != "REFERENCE" || v["direction"] != "DESC" {
		t.Errorf("variables = %v", v)
	}
	filters, _ := json.Marshal(v["filters"])
	var fl struct {
		Reference struct{ Operation, Value string }
		Status    struct{ Include []string }
		CreatedAt struct{ Since, Until *string }
	}
	_ = json.Unmarshal(filters, &fl)
	if fl.Reference.Operation != "CONTAINS" || fl.Reference.Value != "RZ" ||
		len(fl.Status.Include) != 1 || fl.Status.Include[0] != "CONFIRMED" ||
		fl.CreatedAt.Since == nil || *fl.CreatedAt.Since != "2026-01-01T00:00:00Z" || fl.CreatedAt.Until != nil {
		t.Errorf("filters = %s", filters)
	}

	if page.Info.Total != 120 || page.Info.NextCursor != "c-2" || len(page.Items) != 1 {
		t.Fatalf("page = %+v", page)
	}
	b := page.Items[0]
	if b.Customer != "Ada Lovelace" || b.Status != "ON_HOLD" || b.Total != (Money{12345, "GBP"}) ||
		b.StartDate.Month() != time.June || b.CreatedAt.Day() != 4 {
		t.Errorf("booking = %+v", b)
	}
}

func TestRezKitBookingsNoFilterSendsNull(t *testing.T) {
	r, f := newTestRezKit(t, map[string]string{"ListBookings": `{"data":{"bookings":{"list":{"edges":[]}}}}`})
	r.SetWorkspace(Workspace{ID: "org-1"})
	page, err := r.Bookings(context.Background(), BookingQuery{Page: PageRequest{Size: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if f.lastVars["filters"] != nil || f.lastVars["sortBy"] != nil || f.lastVars["after"] != nil {
		t.Errorf("variables = %v", f.lastVars)
	}
	if page.Info.Total != -1 || page.Info.HasNext() {
		t.Errorf("info = %+v", page.Info)
	}
}

const bookingJSON = `{"data":{"bookings":{"booking":{
	"id": "bk-1", "reference": "RZ1", "status": "CONFIRMED", "description": null, "currency": "GBP",
	"tags": ["vip"], "createdAt": "2026-03-04T05:06:07Z", "updatedAt": "2026-03-05T05:06:07Z",
	"customer": {"honorifics": "Ms", "givenName": "Ada", "familyName": "Lovelace", "email": "ada@example.com",
		"telephone": null, "address": {"line1": "12 Marylebone Road", "line2": null, "line3": null,
		"city": "London", "state": null, "postalCode": "NW1 5LR", "country": "GB"}},
	"dates": {"start": "2026-06-01T00:00:00Z", "end": "2026-06-08T00:00:00Z"},
	"totalPrice": {"amount": 10000, "currency": "GBP"},
	"totalPaid": {"amount": 2500, "currency": "GBP"},
	"outstandingBalance": {"amount": 7500, "currency": "GBP"},
	"passengers": [
		{"id": "p1", "honorifics": "Ms", "givenName": "Ada", "familyName": "Lovelace", "email": null,
		 "dateOfBirth": "1985-12-10", "cancelledAt": null},
		{"id": "p2", "honorifics": null, "givenName": "Charles", "familyName": "Babbage", "email": null,
		 "dateOfBirth": null, "cancelledAt": "2026-04-01T10:00:00Z"}],
	"productReservations": [
		{"__typename": "PackageReservation", "id": "r2", "reference": "PR2", "description": null,
		 "status": "ON_HOLD", "statusMessage": "Awaiting supplier", "quantity": 1,
		 "serviceStart": "2026-06-03T00:00:00Z", "serviceEnd": null, "balanceDue": null, "price": null,
		 "product": {"__typename": "Package", "name": "Glacier hike", "category": "Activity"},
		 "passengers": [{"id": "p1"}]},
		{"__typename": "TransportReservation", "id": "r1", "reference": "PR1", "description": null,
		 "status": "CONFIRMED", "statusMessage": null, "quantity": 2,
		 "serviceStart": "2026-06-01T00:00:00Z", "serviceEnd": "2026-06-01T00:00:00Z",
		 "balanceDue": "2026-05-01T00:00:00Z", "price": {"amount": 10000, "currency": "GBP"},
		 "product": {"__typename": "Transport", "name": "Flight", "category": "Flight",
				"element": "Economy", "groupName": "Long haul", "provider": {"name": "SkyLink"}},
		 "passengers": [{"id": "p1"}, {"id": "p2"}],
		 "slices": [
			{"id": "sp3", "ticketClass": null, "ticketNumber": null, "seatAllocation": null, "passenger": {"id": "p1"}, "slice": {"id": "s2", "name": null, "mode": "Road", "intermediateStop": false, "carrier": {"name": "Alpine Transfers", "code": null}, "departure": {"timestamp": "2026-06-01T10:00:00Z", "stop": {"code": null, "name": "Geneva Airport", "city": null, "timezone": "Europe/Zurich"}}, "arrival": {"timestamp": "2026-06-01T13:30:00Z", "stop": {"code": null, "name": "Zermatt", "city": null, "timezone": "Not/AZone"}}}},
			{"id": "sp1", "ticketClass": "Economy", "ticketNumber": "125-1", "seatAllocation": "12A", "passenger": {"id": "p1"}, "slice": {"id": "s1", "name": "728", "mode": "Air", "intermediateStop": false, "carrier": {"name": "British Airways", "code": "BA"}, "departure": {"timestamp": "2026-06-01T06:25:00Z", "stop": {"code": "LHR", "name": "Heathrow", "city": "London", "timezone": "Europe/London"}}, "arrival": {"timestamp": "2026-06-01T08:05:00Z", "stop": {"code": "GVA", "name": "Geneva", "city": null, "timezone": "Europe/Zurich"}}}},
			{"id": "sp2", "ticketClass": "Economy", "ticketNumber": "125-2", "seatAllocation": "12B", "passenger": {"id": "p2"}, "slice": {"id": "s1", "name": "728", "mode": "Air", "intermediateStop": false, "carrier": {"name": "British Airways", "code": "BA"}, "departure": {"timestamp": "2026-06-01T06:25:00Z", "stop": {"code": "LHR", "name": "Heathrow", "city": "London", "timezone": "Europe/London"}}, "arrival": {"timestamp": "2026-06-01T08:05:00Z", "stop": {"code": "GVA", "name": "Geneva", "city": null, "timezone": "Europe/Zurich"}}}}]}],
	"transactions": null,
	"notes": []
}}}}`

func TestRezKitBookingDetail(t *testing.T) {
	r, f := newTestRezKit(t, map[string]string{"GetBooking": bookingJSON})
	d, err := r.Booking(context.Background(), "bk-1")
	if err != nil {
		t.Fatal(err)
	}
	if f.lastVars["id"] != "bk-1" {
		t.Errorf("id var = %v", f.lastVars["id"])
	}
	if d.Reference != "RZ1" || d.Outstanding != (Money{7500, "GBP"}) || d.Start.Day() != 1 {
		t.Errorf("booking = %+v", d)
	}
	if c := d.Customer; c == nil || c.Name() != "Ms Ada Lovelace" ||
		strings.Join(c.Address, "|") != "12 Marylebone Road|London NW1 5LR|GB" {
		t.Errorf("customer = %+v", d.Customer)
	}
	if len(d.Passengers) != 2 || d.Passengers[1].Cancelled.IsZero() || d.Passengers[0].AgeAt(d.Start) != 40 {
		t.Errorf("passengers = %+v", d.Passengers)
	}
	// Sorted by start date; passenger IDs join back to passengers.
	if len(d.Reservations) != 2 || d.Reservations[0].Reference != "PR1" {
		t.Fatalf("reservations = %+v", d.Reservations)
	}
	r0, r1 := d.Reservations[0], d.Reservations[1]
	if r1.Element != "" || r1.Supplier != "" { // absent product fields stay empty
		t.Errorf("reservation 1 = %+v", r1)
	}
	if r0.Category != "Flight" || r0.Element != "Economy" || r0.Group != "Long haul" || r0.Supplier != "SkyLink" ||
		r0.Price == nil || r0.Price.Minor != 10000 || len(r0.PassengerIDs) != 2 {
		t.Errorf("reservation 0 = %+v", r0)
	}
	// Per-passenger slice rows group into legs, sorted by departure, with
	// times on each stop's own clock (unknown zones left as sent).
	if len(r0.Legs) != 2 || len(r1.Legs) != 0 {
		t.Fatalf("legs = %+v / %+v", r0.Legs, r1.Legs)
	}
	air, road := r0.Legs[0], r0.Legs[1]
	if air.Number() != "BA 728" || air.From.Label() != "LHR" || len(air.Passengers) != 2 ||
		air.Passengers[1].Seat != "12B" || air.Depart.Format("15:04") != "07:25" || air.Arrive.Format("15:04") != "10:05" {
		t.Errorf("air leg = %+v (depart %s, arrive %s)", air, air.Depart, air.Arrive)
	}
	if road.Mode != "Road" || road.Number() != "" || road.From.Label() != "Geneva Airport" ||
		road.Depart.Format("15:04") != "12:00" || road.Arrive.Format("15:04") != "13:30" {
		t.Errorf("road leg = %+v (depart %s, arrive %s)", road, road.Depart, road.Arrive)
	}
	if p, ok := d.Passenger(r1.PassengerIDs[0]); !ok || p.Given != "Ada" {
		t.Errorf("passenger join failed: %+v", r1.PassengerIDs)
	}
	if r1.Price != nil || r1.StatusMessage != "Awaiting supplier" || !r1.End.IsZero() {
		t.Errorf("reservation 1 = %+v", r1)
	}
	// null = withheld; [] = none.
	if !d.TransactionsDenied() || d.NotesDenied() || len(d.Notes) != 0 {
		t.Errorf("transactions denied=%v notes=%#v", d.TransactionsDenied(), d.Notes)
	}
}

func TestCivilKeepsDateOnlyValuesInUTC(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip(err)
	}
	old := time.Local
	time.Local = la
	defer func() { time.Local = old }()
	midnight := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	if got := Civil(midnight).Format("2006-01-02"); got != "2026-07-11" {
		t.Errorf("date-only = %s", got)
	}
}

func TestRezKitBookingNotFound(t *testing.T) {
	r, _ := newTestRezKit(t, map[string]string{"GetBooking": `{"data":{"bookings":{"booking":null}}}`})
	if _, err := r.Booking(context.Background(), "nope"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("err = %v", err)
	}
}

func TestFakePaginationFilterSort(t *testing.T) {
	f := NewFake()
	f.SetWorkspace(Workspace{ID: "ws-alpine"})
	ctx := context.Background()

	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		p, err := f.Bookings(ctx, BookingQuery{Page: PageRequest{Size: 50, Cursor: cursor}})
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range p.Items {
			if seen[b.ID] {
				t.Fatalf("duplicate %s across pages", b.ID)
			}
			seen[b.ID] = true
		}
		pages++
		if !p.Info.HasNext() {
			break
		}
		cursor = p.Info.NextCursor
	}
	if len(seen) != 237 || pages != 5 {
		t.Errorf("saw %d bookings over %d pages", len(seen), pages)
	}

	p, _ := f.Bookings(ctx, BookingQuery{
		Filter: BookingFilter{Status: "CONFIRMED"},
		Sort:   Sort{Field: SortReference, Desc: true},
		Page:   PageRequest{Size: 500},
	})
	if len(p.Items) == 0 {
		t.Fatal("no confirmed bookings")
	}
	for i, b := range p.Items {
		if b.Status != "CONFIRMED" {
			t.Fatalf("filter leaked status %q", b.Status)
		}
		if i > 0 && b.Reference > p.Items[i-1].Reference {
			t.Fatalf("not sorted desc by reference at %d", i)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	if got := StatusLabel("ON_HOLD"); got != "On hold" {
		t.Errorf("got %q", got)
	}
}
