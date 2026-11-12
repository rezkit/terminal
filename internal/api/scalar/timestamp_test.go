package scalar

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimestampDecode(t *testing.T) {
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	for _, in := range []string{
		`"2026-03-04T05:06:07Z"`, `"2026-03-04T05:06:07.000Z"`, `"2026-03-04T06:06:07+01:00"`,
		`1772600767`, `1772600767000`, `"1772600767000"`,
	} {
		var ts Timestamp
		if err := json.Unmarshal([]byte(in), &ts); err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if !ts.Equal(want) {
			t.Errorf("%s: got %v", in, ts.Time)
		}
	}
	var ts Timestamp
	if err := json.Unmarshal([]byte(`"2026-03-04"`), &ts); err != nil || ts.Day() != 4 {
		t.Errorf("date only: %v %v", ts, err)
	}
	if err := json.Unmarshal([]byte(`null`), &ts); err != nil || !ts.IsZero() {
		t.Errorf("null: %v %v", ts, err)
	}
	if err := json.Unmarshal([]byte(`"yesterday"`), &ts); err == nil {
		t.Error("garbage accepted")
	}
}

func TestTimestampEncode(t *testing.T) {
	b, _ := json.Marshal(Timestamp{time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)})
	if string(b) != `"2026-03-04T05:06:07Z"` {
		t.Errorf("got %s", b)
	}
}
