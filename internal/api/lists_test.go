package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRezKitCustomersVariablesAndMapping(t *testing.T) {
	r, f := newTestRezKit(t, map[string]string{"ListCustomers": `{"data":{"customers":{"list":{
		"totalCount": 41,
		"pageInfo": {"hasNextPage": true, "endCursor": "c-9"},
		"edges": [{"node": {
			"id": "cu-1", "honorifics": "Dr", "givenName": "Ada", "familyName": "Lovelace",
			"email": "ada@example.com", "telephone": null, "createdAt": "2026-03-04T05:06:07Z",
			"address": {"city": "London", "country": "GB"}
		}}, {"node": {
			"id": "cu-2", "givenName": "Alan", "familyName": "Turing", "email": "alan@example.com",
			"createdAt": "2026-03-05T00:00:00Z", "address": null
		}}, null]
	}}}}`})
	r.SetWorkspace(Workspace{ID: "org-1"})
	page, err := r.Customers(context.Background(), CustomerQuery{
		Filter: CustomerFilter{Family: "love", Email: "example"},
		Sort:   Sort{Field: SortFamilyName},
		Page:   PageRequest{Size: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	v := f.lastVars
	if v["organizationId"] != "org-1" || v["first"] != float64(20) || v["after"] != nil ||
		v["sortBy"] != "FAMILY_NAME" || v["direction"] != "ASC" {
		t.Errorf("variables = %v", v)
	}
	filters, _ := json.Marshal(v["filters"])
	if s := string(filters); !strings.Contains(s, `"familyName":{"operation":"CONTAINS","value":"love"}`) ||
		!strings.Contains(s, `"email":{"operation":"CONTAINS","value":"example"}`) ||
		!strings.Contains(s, `"givenName":null`) {
		t.Errorf("filters = %s", s)
	}
	if page.Info.Total != 41 || page.Info.NextCursor != "c-9" || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if c := page.Items[0]; c.Name() != "Dr Ada Lovelace" || c.Location() != "London, GB" || c.Telephone != "" {
		t.Errorf("customer = %+v", c)
	}
	if c := page.Items[1]; c.Location() != "" {
		t.Errorf("customer without address = %+v", c)
	}
}

func TestRezKitCustomerDetail(t *testing.T) {
	r, _ := newTestRezKit(t, map[string]string{"GetCustomer": `{"data":{"customers":{"customer":{
		"id": "cu-1", "honorifics": null, "givenName": "Ada", "familyName": "Lovelace", "knownAs": "Ada",
		"email": "ada@example.com", "telephone": "+44 20 7946 0000", "dateOfBirth": "1815-12-10T00:00:00Z",
		"sex": "FEMALE", "tags": ["vip"],
		"createdAt": "2026-01-01T10:00:00Z", "updatedAt": "2026-02-01T10:00:00Z",
		"address": {"line1": "12 St James's Square", "line2": null, "line3": null, "city": "London",
			"state": null, "postalCode": "SW1Y 4JH", "country": "GB"},
		"bookings": [
			{"id": "b1", "reference": "OLD", "status": "CONFIRMED", "description": null,
				"createdAt": "2025-01-01T00:00:00Z", "dates": {"start": "2025-06-01T00:00:00Z", "end": null},
				"totalPrice": {"amount": 1000, "currency": "GBP"}, "outstandingBalance": {"amount": 0, "currency": "GBP"}},
			{"id": "b2", "reference": "NEW", "status": "PENDING", "description": "Alps",
				"createdAt": "2026-01-01T00:00:00Z", "dates": {"start": "2026-08-01T00:00:00Z", "end": "2026-08-08T00:00:00Z"},
				"totalPrice": {"amount": 5000, "currency": "GBP"}, "outstandingBalance": {"amount": 4000, "currency": "GBP"}},
			{"id": "b3", "reference": "UNDATED", "status": "PENDING", "description": null,
				"createdAt": "2025-09-01T00:00:00Z", "dates": {"start": null, "end": null},
				"totalPrice": {"amount": 0, "currency": "GBP"}, "outstandingBalance": {"amount": 0, "currency": "GBP"}}
		],
		"notes": null
	}}}}`})
	d, err := r.Customer(context.Background(), "cu-1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "Ada Lovelace" || d.KnownAs != "Ada" || d.Sex != "FEMALE" || d.DateOfBirth.Year() != 1815 ||
		strings.Join(d.Address, "|") != "12 St James's Square|London SW1Y 4JH|GB" {
		t.Errorf("detail = %+v", d)
	}
	var refs []string
	for _, b := range d.Bookings {
		refs = append(refs, b.Reference)
	}
	if strings.Join(refs, ",") != "NEW,UNDATED,OLD" { // newest first; undated by creation
		t.Errorf("booking order = %v", refs)
	}
	if b := d.Bookings[0]; b.Outstanding != (Money{4000, "GBP"}) || b.End.Day() != 8 || b.Description != "Alps" {
		t.Errorf("booking = %+v", b)
	}
	if d.BookingsDenied() || !d.NotesDenied() {
		t.Errorf("denied: bookings %v notes %v", d.BookingsDenied(), d.NotesDenied())
	}
}

func TestRezKitCustomerWithheldBookings(t *testing.T) {
	r, _ := newTestRezKit(t, map[string]string{"GetCustomer": `{"data":{"customers":{"customer":{
		"id": "cu-1", "givenName": "Ada", "familyName": "Lovelace", "email": "a@example.com",
		"createdAt": "2026-01-01T10:00:00Z", "updatedAt": "2026-02-01T10:00:00Z",
		"bookings": null, "notes": []}}}}`})
	d, err := r.Customer(context.Background(), "cu-1")
	if err != nil {
		t.Fatal(err)
	}
	if !d.BookingsDenied() || d.NotesDenied() || len(d.Address) != 0 {
		t.Errorf("detail = %+v", d)
	}
	r2, _ := newTestRezKit(t, map[string]string{"GetCustomer": `{"data":{"customers":{"customer":null}}}`})
	if _, err := r2.Customer(context.Background(), "nope"); err == nil {
		t.Error("expected not found")
	}
}

func TestRezKitTransactionsVariablesAndMapping(t *testing.T) {
	r, f := newTestRezKit(t, map[string]string{"ListTransactions": `{"data":{"transactions":{
		"totalCount": 3,
		"pageInfo": {"hasNextPage": false, "endCursor": "c-3"},
		"edges": [{"node": {
			"id": "tx-1", "reference": "PAY-1", "description": "Deposit", "category": "CARD", "status": "CONFIRMED",
			"createdAt": "2026-03-04T05:06:07Z", "amount": {"amount": 2500, "currency": "EUR"},
			"booking": {"id": "bk-1", "reference": "RZ1", "customer": {"givenName": "Ada", "familyName": "Lovelace"}}
		}}, {"node": {
			"id": "tx-2", "reference": null, "description": null, "category": "CASH", "status": "VOID",
			"createdAt": "2026-03-05T00:00:00Z", "amount": null,
			"booking": {"id": "bk-2", "reference": "RZ2", "customer": null}
		}}]
	}}}`})
	r.SetWorkspace(Workspace{ID: "org-1"})
	to := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	page, err := r.Transactions(context.Background(), TransactionQuery{
		Filter: TransactionFilter{BookingRef: "RZ", Currency: "eur", CreatedTo: to},
		Page:   PageRequest{Size: 10, Cursor: "c-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	v := f.lastVars
	if v["organizationId"] != "org-1" || v["first"] != float64(10) || v["after"] != "c-1" {
		t.Errorf("variables = %v", v)
	}
	if _, ok := v["sortBy"]; ok {
		t.Errorf("transactions take no sort: %v", v)
	}
	filters, _ := json.Marshal(v["filters"])
	if s := string(filters); !strings.Contains(s, `"booking":{`) ||
		!strings.Contains(s, `"reference":{"operation":"CONTAINS","value":"RZ"}`) ||
		!strings.Contains(s, `"amountCurrency":{"operation":"INCLUDE","values":["EUR"]}`) ||
		!strings.Contains(s, `"until":"2026-04-01T00:00:00Z"`) {
		t.Errorf("filters = %s", s)
	}
	if page.Info.Total != 3 || page.Info.HasNext() || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if tx := page.Items[0]; tx.BookingID != "bk-1" || tx.BookingRef != "RZ1" || tx.Customer != "Ada Lovelace" ||
		tx.Amount == nil || *tx.Amount != (Money{2500, "EUR"}) || tx.Reference != "PAY-1" {
		t.Errorf("tx = %+v", tx)
	}
	if tx := page.Items[1]; tx.Amount != nil || tx.Customer != "" || tx.Status != "VOID" {
		t.Errorf("tx = %+v", tx)
	}
}

func TestFakeCustomersAndTransactions(t *testing.T) {
	f := NewFake()
	f.SetWorkspace(Workspace{ID: "ws-alpine"})
	ctx := context.Background()

	page, err := f.Customers(ctx, CustomerQuery{Sort: Sort{Field: SortFamilyName}, Page: PageRequest{Size: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) < 30 || page.Items[0].Family > page.Items[1].Family {
		t.Fatalf("customers = %d, first %+v", len(page.Items), page.Items[0])
	}
	filtered, _ := f.Customers(ctx, CustomerQuery{Filter: CustomerFilter{Family: "LOVE"}, Page: PageRequest{Size: 100}})
	if len(filtered.Items) != 1 || filtered.Items[0].Given != "Ada" {
		t.Fatalf("filtered = %+v", filtered.Items)
	}

	// Ada has bookings; each opens, and the transactions list links back to them.
	d, err := f.Customer(ctx, filtered.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Bookings) == 0 || d.BookingsDenied() {
		t.Fatalf("Ada has no bookings: %+v", d)
	}
	if _, err := f.Booking(ctx, d.Bookings[0].ID); err != nil {
		t.Fatal(err)
	}
	txs, err := f.Transactions(ctx, TransactionQuery{
		Filter: TransactionFilter{BookingRef: d.Bookings[0].Reference}, Page: PageRequest{Size: 50}})
	if err != nil {
		t.Fatal(err)
	}
	if len(txs.Items) < 2 || txs.Items[0].BookingID != d.Bookings[0].ID || txs.Items[0].Customer != "Ada Lovelace" {
		t.Fatalf("transactions = %+v", txs.Items)
	}
	for i := 1; i < len(txs.Items); i++ {
		if txs.Items[i].Created.After(txs.Items[i-1].Created) {
			t.Fatal("transactions not newest first")
		}
	}
	all, _ := f.Transactions(ctx, TransactionQuery{Page: PageRequest{Size: 10}})
	if all.Info.Total < 237*2 || !all.Info.HasNext() {
		t.Errorf("all transactions info = %+v", all.Info)
	}
}
