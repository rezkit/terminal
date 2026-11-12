package api

import "time"

// TransactionRow is a row in the transactions list: a payment and the
// booking it belongs to.
type TransactionRow struct {
	Transaction
	BookingID, BookingRef, Customer string
}

// TransactionFilter narrows the transactions list. Zero values mean "any".
// The API cannot filter transactions by status.
type TransactionFilter struct {
	Reference   string    // transaction reference contains
	BookingRef  string    // booking reference contains
	Currency    string    // ISO code, exact
	CreatedFrom time.Time // created on/after
	CreatedTo   time.Time // created on/before
}

// TransactionQuery is a full transactions list request. The API has no sort
// argument for transactions, so there is no Sort.
type TransactionQuery struct {
	Filter TransactionFilter
	Page   PageRequest
}
