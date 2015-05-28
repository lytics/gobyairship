package gobyairship_test

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("%d == %s", hits, r.Header.Get("Cookie"))
		if hits != 0 {
			if cval, err := strconv.Atoi(r.Header.Get("Cookie")); err != nil || cval != hits {
				t.Logf("Error retrieving cookie %d after redirect: %v", cval, err)
				w.WriteHeader(500)
				return
			}
		}
		hits++
		// Just a 307 should be enough to trigger redirect logic
		w.Header().Add("Set-Cookie", strconv.Itoa(hits))
		w.WriteHeader(307)
	}))
	defer ts.Close()

	c := NewClient("", "")
	c.BaseURL = ts.URL

	// Test with and without a request body
	for _, body := range [][]byte{nil, []byte("{}")} {
		hits = 0
		resp, err := c.Post("events", body)
		if resp != nil {
			t.Fatalf("Expected response to be nil; status code=%d", resp.StatusCode)
		}
		if err != ErrTooManyRedirects {
			t.Fatalf("Expected TooManyRedirects error, but found err==%v", err)
		}
	}
}

// TestGzip ensures the client accepts gzip encoded responses.
func TestGzip(t *testing.T) {
	var sz int64 = 10 * 1000 * 1000
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			t.Logf("'Accept-Encoding: gzip' header not sent: %q", r.Header.Get("Accept-Encoding"))
			w.WriteHeader(500)
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Log("Missing Authorization header")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.urbanairship+x‐ndjson;version=3;")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("UA-Operation-Id", "test-id")
		w.WriteHeader(200)
		gzw := gzip.NewWriter(w)
		val := []byte("1234567890")
		for i := int64(0); i < sz; i += 10 {
			if n, err := gzw.Write(val); n != 10 {
				t.Logf("Wrote %d bytes; expected to write %d. Error: %v", n, sz, err)
				return
			}
		}
		if err := gzw.Close(); err != nil {
			t.Logf("Error closing gzip writer: %v", err)
		}
	}))
	defer ts.Close()

	c := NewClient("", "")
	c.BaseURL = ts.URL

	resp, err := c.Post("", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Non-200 status code: %d", resp.StatusCode)
	}

	// Cannot assert Content-Encoding == gzip because Go's http library handles
	// ungzipping the response and strips the header to avoid clients
	// double-ungzipping.

	if len(resp.TransferEncoding) == 0 || resp.TransferEncoding[0] != "chunked" {
		t.Fatalf("Expected chunk transfer encoding, found: %v", resp.TransferEncoding)
	}

	n, err := io.CopyN(ioutil.Discard, resp.Body, sz)
	if n != sz {
		t.Fatalf("Read %d bytes; expected to read %d. Error: %v", n, sz, err)
	}
}
