// Package api defines the data the UI needs and the clients that provide it.
//
// The UI depends only on the Service interface and the plain types below.
// The GraphQL implementation (genqlient, generated from the introspected
// schema) adapts generated types into these, so schema details never leak
// into the UI, and tests/demo mode can use the in-memory Fake.
package api

import (
	"context"
	"strings"
	"time"
)

// Workspace is an organization workspace the user can act in.
type Workspace struct {
	ID          string
	Name        string
	Environment string // e.g. Production, Sandbox
}

// Sort orders a listing by a field (an API sort key).
type Sort struct {
	Field string
	Desc  bool
}

// PageRequest asks for one page. Cursor is opaque: empty means the first
// page; otherwise a value previously returned in PageInfo.NextCursor.
type PageRequest struct {
	Size   int
	Cursor string
}

// PageInfo describes the page returned.
type PageInfo struct {
	NextCursor string // empty when there is no next page
	Total      int    // -1 when the API does not report a total
}

// HasNext reports whether another page follows.
func (p PageInfo) HasNext() bool { return p.NextCursor != "" }

// Page is one page of results.
type Page[T any] struct {
	Items []T
	Info  PageInfo
}

// Money is an amount in minor units with an ISO currency code.
type Money struct {
	Minor    int64
	Currency string
}

// Field is one labelled value on a detail view.
type Field struct {
	Label string
	Value string
}

// Section groups fields on a detail view.
type Section struct {
	Title  string
	Fields []Field
}

// Detail is a schema-agnostic view of a single resource.
type Detail struct {
	ID       string
	Title    string
	Sections []Section
}

// Booking is a row in the bookings list.
type Booking struct {
	ID          string
	Reference   string
	Status      string // API enum value, e.g. ON_HOLD; see StatusLabel
	Description string
	Customer    string
	StartDate   time.Time // zero when the booking has no dated services
	Total       Money
	CreatedAt   time.Time
}

// BookingFilter narrows the bookings list. Zero values mean "any".
type BookingFilter struct {
	Reference   string    // reference contains
	Status      string    // API enum value
	CreatedFrom time.Time // created on/after
	CreatedTo   time.Time // created on/before (inclusive day when date-only)
}

// BookingQuery is a full bookings list request.
type BookingQuery struct {
	Filter BookingFilter
	Sort   Sort
	Page   PageRequest
}

// Service is everything the UI reads from RezKit.
type Service interface {
	// Workspaces lists the workspaces available to the current user.
	Workspaces(ctx context.Context) ([]Workspace, error)
	// SetWorkspace selects the workspace subsequent calls act in.
	SetWorkspace(w Workspace)

	Bookings(ctx context.Context, q BookingQuery) (Page[Booking], error)
	Booking(ctx context.Context, id string) (BookingDetail, error)

	Customers(ctx context.Context, q CustomerQuery) (Page[CustomerRow], error)
	Customer(ctx context.Context, id string) (CustomerDetail, error)

	Transactions(ctx context.Context, q TransactionQuery) (Page[TransactionRow], error)
}

// Sort keys for bookings, as supported server-side by the API.
const (
	SortReference   = "reference"
	SortDescription = "description"
	SortCreatedAt   = "createdAt"
	SortUpdatedAt   = "updatedAt"
)

// BookingStatuses are the BookingStatus enum values from the schema.
var BookingStatuses = []string{
	"PENDING", "PROCESSING", "ON_HOLD", "WAITLISTED", "CONFIRMED",
	"CANCELLING", "CANCELLED", "ARCHIVED", "ERROR",
}

// StatusLabel turns an enum value like ON_HOLD into "On hold".
func StatusLabel(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ToLower(strings.ReplaceAll(s, "_", " "))
	return strings.ToUpper(s[:1]) + s[1:]
}

var (
	_ Service = (*RezKit)(nil)
	_ Service = (*Fake)(nil)
)
