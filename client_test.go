package gobyairship_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/lytics/gobyairship"
)

// TestPostRedirectCookie ensures that the default Client properly sets cookies
// and follows redirects as required by some Urban Airship APIs.
func TestPostRedirectCookie(t *testing.T) {
	t.Parallel()

	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch hits {
		case 1:
			// On the first hit, redirect with a Set-Cookie header as per
			// /api/events/ spec.
			w.Header().Add("Set-Cookie", "testcookie")
			w.Header().Add("Location", "/foo")
			w.WriteHeader(307)
		case 2, 3, 4:
			if r.Header.Get("Cookie") != "testcookie" {
				w.WriteHeader(500)
				return
			}
			w.Header().Add("Set-Cookie", "testcookie")
			w.Header().Add("Location", "/foo")
			w.WriteHeader(307)
		case 5:
			if r.Header.Get("Cookie") != "testcookie" {
				t.Logf("Wrong Cookie header: %#v", r.Header)
				w.WriteHeader(500)
			}
			w.WriteHeader(200)
		default:
			w.WriteHeader(500)
		}
	}))
	defer ts.Close()

	c := NewClient("", "")
	c.BaseURL = ts.URL
	resp, err := c.Post("events", nil)
	if err != nil {
		t.Fatalf("Unexpected error POSTing to test server: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Unexpected status code, did client not handle cookie? %d", resp.StatusCode)
	}
}

// TestTooManyRedirects ensures that the Client.Post method doesn't follow
// redirects forever.
func TestTooManyRedirects(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just a 307 should be enough to trigger redirect logic
		w.WriteHeader(307)
	}))
	defer ts.Close()
	c := NewClient("", "")
	c.BaseURL = ts.URL

	// Test with and without a request body
	for _, body := range [][]byte{nil, []byte("{}")} {
		resp, err := c.Post("events", body)
		if err != ErrTooManyRedirects {
			t.Fatalf("Expected TooManyRedirects error, but found err==%q", err)
		}
		if resp != nil {
			t.Fatal("Expected response to be nil")
		}
	}
}
