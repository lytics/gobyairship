package events_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/lytics/gobyairship/events"
)

var (
	// Base dir to find test data files, override with TEST_EVENTS_PATH
	testDataPath = "${GOPATH}/src/github.com/lytics/gobyairship/events/testdata"
)

func init() {
	if os.Getenv("TEST_EVENTS_PATH") != "" {
		testDataPath = os.Getenv("TEST_EVENTS_PATH")
	}
}

// fakeClient impelemnts the Client interface but is backed by a fixture
// instead of the real Urban Airship API.
type fakeClient struct {
	filter events.Type
	data   io.ReadCloser
}

func (c *fakeClient) Post(url string, body interface{}) (*http.Response, error) {
	// body should be a Request; validate it
	req, ok := body.(*events.Request)
	if !ok {
		return nil, fmt.Errorf("body is not a Request: %T", body)
	}
	if len(req.Filters) == 0 && c.filter != "all" {
		return nil, fmt.Errorf("expected filter=%q but no filter specified", c.filter)
	}
	if len(req.Filters) > 1 || len(req.Filters[0].Types) != 1 {
		return nil, fmt.Errorf("expected filter=%q but received filter=%v", c.filter, req.Filters)
	}
	if req.Filters[0].Types[0] != c.filter {
		return nil, fmt.Errorf("expected filter=%q but received filter=%q", c.filter, req.Filters[0].Types[0])
	}
	return &http.Response{StatusCode: 200, Body: c.data}, nil
}

// filter type test files
var filterTypes = map[string]events.Type{
	"all":        "",
	"close":      events.TypeClose,
	"first_open": events.TypeFirst,
	"location":   events.TypeLocation,
	"open":       events.TypeOpen,
	"send":       events.TypeSend,
	"tag_change": events.TypeTagChange,
	"uninstall":  events.TypeUninstall,
}

func TestFilterTypes(t *testing.T) {
	t.Parallel()
	for fname, ftype := range filterTypes {
		fn := fmt.Sprintf("%s/%s.json", os.ExpandEnv(testDataPath), fname)
		f, err := os.Open(fn)
		if err != nil {
			t.Fatalf("Error opening filter file %q: %v", fn, err)
		}
		fc := &fakeClient{filter: ftype, data: f}

		offset := uint64(0)
		resp, err := events.Fetch(fc, events.StartFirst, offset, []*events.Filter{{Types: []events.Type{ftype}}})
		if err != nil {
			t.Errorf("Received error fetching %s: %v", fname, err)
			continue
		}

		// Check all events
		for ev := range resp.Events() {
			if ev.Offset < offset {
				t.Error("%s - Expected offset to monotonically increase: %d < %d", fname, ev.Offset, offset)
				continue
			}
			offset = ev.Offset
			if msg := checkEvent(ftype, ev); msg != "" {
				t.Errorf("%s - %s\n%s", fname, msg, string(ev.Body))
			}
		}
		if resp.Err() != io.EOF {
			t.Errorf("Received error iterating over events for %s: %v", fname, err)
		}
	}
}

func checkEvent(ft events.Type, ev *events.Event) (errmsg string) {
	if ev.ID == "" {
		return "Missing ID"
	}
	if ev.Type != ft {
		return fmt.Sprintf("Expected type %s but found %s", ft, ev.Type)
	}
	if ev.Occurred.IsZero() || ev.Occurred.After(time.Now()) {
		return fmt.Sprintf("Invalid Occurred timestamp: %s", ev.Occurred)
	}
	if ev.Processed.IsZero() || ev.Processed.After(time.Now()) {
		return fmt.Sprintf("Invalid Processed timestamp: %s", ev.Processed)
	}
	if ev.Occurred.After(ev.Processed) {
		return fmt.Sprintf("Occurred after Processed?! %s > %s", ev.Occurred, ev.Processed)
	}
	switch ft {
	case events.TypePush:
		push, err := ev.Push()
		if err != nil {
			return err.Error()
		}
		if push.PushID == "" {
			return "Empty push ID"
		}
		if len(push.Payload) == 0 {
			return "Empty payload"
		}
	case events.TypeOpen:
		open, err := ev.Open()
		if err != nil {
			return err.Error()
		}
		if open.LastDelivered == "" {
			return "Empty last delivered"
		}
	case events.TypeSend:
		send, err := ev.Send()
		if err != nil {
			return err.Error()
		}
		if send.PushID == "" {
			return "Empty push ID"
		}
	case events.TypeClose:
		closeev, err := ev.Close()
		if err != nil {
			return err.Error()
		}
		if closeev.SessionID == "" {
			return "Empty session ID"
		}
	case events.TypeTagChange:
		tagc, err := ev.TagChange()
		if err != nil {
			return err.Error()
		}
		if len(tagc.Add)+len(tagc.Remove) == 0 {
			return "Tag change without any tag changes"
		}
		if len(tagc.Remove) == 0 && len(tagc.Current) == 0 {
			return "No tags yet no removals"
		}
	case events.TypeLocation:
		loc, err := ev.Location()
		if err != nil {
			return err.Error()
		}
		if loc.Lat == "" {
			return "No lat"
		}
		if loc.Lon == "" {
			return "No lon"
		}
		if _, err := loc.Lat.Float64(); err != nil {
			return "Error getting float form of lat: " + err.Error()
		}
		if _, err := loc.Lon.Float64(); err != nil {
			return "Error getting float form of lon: " + err.Error()
		}
	case events.TypeCustom, events.TypeFirst, events.TypeUninstall:
		// Nothing to do for these events
	default:
		return "Unsupported type: " + string(ft)
	}
	return ""
}
