package gobyairship

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
)

const DefaultBaseURL = "https://api.urbanairship.com/api"

var ErrTooManyRedirects = errors.New("too many redirects")

// Client is an Urban Airship API client. It handles authentication and
// provides helpers for making requests against the API.
type Client struct {
	// BaseURL is the base location of the Urban Airship API and defaults to
	// DefaultBaseURL.
	BaseURL string

	// HTTPClient is the *http.Client to use when making requests. It defaults to
	// http.DefaultClient.
	HTTPClient *http.Client

	key    string
	secret string
}

// NewClient creates a new Urban Airship API Client using the given App Key and
// Master Secret.
func NewClient(key, secret string) *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: http.DefaultClient,
		key:        key,
		secret:     secret,
	}
}

// Post a request to the Urban Airship API with the Client's credentials. If
// body is non-nil it is marshaled to JSON and the appropriate headers are set.
func (c *Client) Post(url string, body interface{}) (*http.Response, error) {
	// Construct full URL
	fullURL := c.BaseURL + "/" + url + "/"

	// Marshal body if it is non-nil
	var buf []byte
	if body != nil {
		var err error
		buf, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := c.newRequest("POST", fullURL, buf)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	// The Urban Airship API may respond with a 307 + Set-Cookie on POSTs which
	// is non-standard and therefore handled by this wrapper method instead of by
	// Go's http.Client. Give up after 10 redirects.
	try := 0
	const tries = 10
	for ; resp.StatusCode == http.StatusTemporaryRedirect && try < tries; try++ {
		// Cleanup body of redirect response so the connection will be reused
		ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		// POST to specified location (if one specified)
		loc := resp.Header.Get("Location")
		if loc == "" {
			// No Location returned, reuse original
			loc = fullURL
		} else if !strings.HasPrefix(loc, "http") {
			// Relative URL
			loc = c.BaseURL + loc
		}

		req, err := c.newRequest("POST", loc, buf)
		if err != nil {
			return nil, err
		}

		// Set the "cookie" token if it's sent
		if cookie := resp.Header.Get("Set-Cookie"); cookie != "" {
			req.Header.Add("Cookie", cookie)
		}
		resp, err = c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
	}
	if try == tries {
		// Exhausted retries; cleanup response and return an error
		ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, ErrTooManyRedirects
	}
	return resp, nil
}

// newRequest adds basic auth and accept headers to an Urban Airship API
// request. If buf is non-nil it is assumed to be JSON.
func (c *Client) newRequest(method, url string, buf []byte) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.key, c.secret)
	req.Header.Set("Accept", "application/vnd.urbanairship+x-json,application/vnd.urbanairship+x-ndjson;version=3;")
	if len(buf) > 0 {
		req.Body = ioutil.NopCloser(bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
