// Package scalar holds Go types bound to custom GraphQL scalars.
package scalar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Timestamp is the RezKit `Timestamp` scalar. The schema does not document
// its wire format, so decoding accepts ISO-8601/RFC 3339 strings, plain
// dates, numeric strings and JSON numbers (epoch seconds or milliseconds).
// Encoding uses RFC 3339.
type Timestamp struct{ time.Time }

var layouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02"}

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*t = Timestamp{}
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*t = Timestamp{}
			return nil
		}
		for _, l := range layouts {
			if v, err := time.Parse(l, s); err == nil {
				t.Time = v
				return nil
			}
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			t.Time = fromEpoch(n)
			return nil
		}
		return fmt.Errorf("unrecognised timestamp %q", s)
	}
	n, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return fmt.Errorf("unrecognised timestamp %s", b)
	}
	t.Time = fromEpoch(n)
	return nil
}

// fromEpoch treats values beyond year ~5138 in seconds as milliseconds.
func fromEpoch(n float64) time.Time {
	if n > 1e11 {
		return time.UnixMilli(int64(n)).UTC()
	}
	return time.Unix(int64(n), 0).UTC()
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.UTC().Format(time.RFC3339))
}
