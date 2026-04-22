package httpclient

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter_Seconds(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   time.Duration
		wantOK bool
	}{
		{"zero", "0", 0, true},
		{"positive", "120", 120 * time.Second, true},
		{"negative", "-1", 0, false},
		{"empty", "", 0, false},
		{"non numeric garbage", "soon", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRetryAfter(tc.input)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("duration = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).UTC()

	// RFC1123 (default HTTP-date format)
	rfc1123 := future.Format(time.RFC1123)
	if d, ok := ParseRetryAfter(rfc1123); !ok || d <= 0 {
		t.Errorf("RFC1123: ok=%v duration=%v (want ok and positive)", ok, d)
	}

	// RFC1123 with numeric TZ
	rfc1123z := future.Format(time.RFC1123Z)
	if d, ok := ParseRetryAfter(rfc1123z); !ok || d <= 0 {
		t.Errorf("RFC1123Z: ok=%v duration=%v (want ok and positive)", ok, d)
	}

	// RFC850 (older)
	rfc850 := future.Format(time.RFC850)
	if _, ok := ParseRetryAfter(rfc850); !ok {
		t.Errorf("RFC850 should be accepted")
	}

	// ANSI C asctime
	ansi := future.Format("Mon Jan _2 15:04:05 2006")
	if _, ok := ParseRetryAfter(ansi); !ok {
		t.Errorf("ANSI asctime should be accepted")
	}
}

func TestParseRetryAfter_HTTPDateInPast(t *testing.T) {
	// Date dans le passé : valide mais durée <= 0 (retry immédiat côté caller).
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC1123)
	d, ok := ParseRetryAfter(past)
	if !ok {
		t.Fatal("past HTTP-date should still be accepted as valid")
	}
	if d > 0 {
		t.Errorf("expected non-positive duration for past date, got %v", d)
	}
}

func TestParseRetryAfter_InvalidFormats(t *testing.T) {
	cases := []string{
		"not a date",
		"2026-01-01",
		"12:34:56",
	}
	for _, v := range cases {
		if _, ok := ParseRetryAfter(v); ok {
			t.Errorf("ParseRetryAfter(%q) should be false", v)
		}
	}
}

func TestRetryAfterFromError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if _, ok := RetryAfterFromError(nil); ok {
			t.Error("nil error should return ok=false")
		}
	})

	t.Run("unrelated error", func(t *testing.T) {
		if _, ok := RetryAfterFromError(errors.New("plain")); ok {
			t.Error("non-*Error should return ok=false")
		}
	})

	t.Run("httpclient.Error with Retry-After", func(t *testing.T) {
		httpErr := &Error{
			StatusCode:      429,
			ResponseHeaders: http.Header{"Retry-After": []string{"42"}},
		}
		wrapped := fmt.Errorf("boom: %w", httpErr)
		d, ok := RetryAfterFromError(wrapped)
		if !ok {
			t.Fatal("expected ok=true for wrapped *Error with Retry-After")
		}
		if d != 42*time.Second {
			t.Errorf("duration = %v, want 42s", d)
		}
	})

	t.Run("httpclient.Error without Retry-After", func(t *testing.T) {
		httpErr := &Error{StatusCode: 500}
		if _, ok := RetryAfterFromError(httpErr); ok {
			t.Error("expected ok=false for *Error without Retry-After header")
		}
	})
}
