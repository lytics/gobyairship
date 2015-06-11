package events_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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

func newFakeClient(t *testing.T, fname string, ftype events.Type) *fakeClient {
	fn := fmt.Sprintf("%s/%s.json", os.ExpandEnv(testDataPath), fname)
	raw, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Fatalf("Error reading filter file %q: %v", fn, err)
	}
	return &fakeClient{filter: ftype, data: ioutil.NopCloser(bytes.NewReader(raw))}
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
	"push_body":  events.TypePush,
}

func TestFilterTypes(t *testing.T) {
	t.Parallel()
	for fname, ftype := range filterTypes {
		t.Log("Testing", fname)
		fc := newFakeClient(t, fname, ftype)

		offset := uint64(0)
		resp, err := events.Fetch(fc, events.StartOffset, 0, nil, &events.Filter{Types: []events.Type{ftype}})
		if err != nil {
			t.Errorf("Received error fetching %s: %v", fname, err)
			continue
		}

		// Check all events
		i := 0
		for ev := range resp.Events() {
			i++
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
		if i == 0 {
			t.Errorf("No events processed for %s", fname)
		}
	}
}

func checkEvent(ft events.Type, ev *events.Event) (errmsg string) {
	if ev.ID == "" {
		return "Missing ID"
	}
	// ft == "" means ev may be of /any/ type
	if ft != "" && ev.Type != ft {
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

	if ev.Device != nil {
		if len(ev.Device.Amazon)+len(ev.Device.Android)+len(ev.Device.IOS)+len(ev.Device.NamedUser) == 0 {
			return "Device specified but no IDs"
		}
	}
	switch ev.Type {
	case events.TypePush:
		push, err := ev.PushBody()
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
		if open.LastReceived != nil && open.LastReceived.PushID == "" {
			return "Empty last received push ID"
		}
		if open.ConvertingPush != nil && open.ConvertingPush.PushID == "" {
			return "Empty converting push ID"
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
		return "Unsupported type: " + string(ev.Type)
	}
	return ""
}

var failClientErr = errors.New("failClient always fails")

type failClient struct{}

func (failClient) Post(string, interface{}) (*http.Response, error) { return nil, failClientErr }

func TestRequestValidate(t *testing.T) {
	t.Parallel()
	c := failClient{}

	// Fetch should only set the offset if the start is StartOffset
	_, err := events.Fetch(c, events.StartFirst, 0, nil, nil)
	if err != failClientErr {
		t.Errorf("unexpected error when setting both start and offset: %v %T %p %p", err, err)
	}

	_, err = events.Fetch(c, "invalid", 0, nil, nil)
	if err == nil || err == failClientErr {
		t.Errorf("expected error when setting invalid start value")
	}

	_, err = events.Fetch(c, events.StartLast, 0, &events.Subset{})
	if err == nil || err == failClientErr {
		t.Errorf("expected error with empty (non-nil) subset")
	}

	_, err = events.Fetch(c, events.StartLast, 0, &events.Subset{Type: "invalid"})
	if err == nil || err == failClientErr {
		t.Errorf("expected error with invalid subset type")
	}

	_, err = events.Fetch(c, events.StartLast, 0, events.SubsetPartition(-1, 99))
	if err == nil || err == failClientErr {
		t.Errorf("expected error with invalid subset partition count")
	}

	_, err = events.Fetch(c, events.StartLast, 0, events.SubsetPartition(10, 99))
	if err == nil || err == failClientErr {
		t.Errorf("expected error with invalid subset partition selection")
	}

	_, err = events.Fetch(c, events.StartLast, 0, events.SubsetSample(99))
	if err == nil || err == failClientErr {
		t.Errorf("expected error with invalid subset sample")
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	fc := newFakeClient(t, "all", "")
	resp, err := events.Fetch(fc, events.StartFirst, 0, nil, &events.Filter{Types: []events.Type{""}})
	if err != nil {
		t.Fatalf("Received error fetching: %v", err)
	}

	// Close should be safe to call all the time
	done := make(chan bool)
	go func() {
		resp.Close()
		close(done)
	}()
	resp.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("Close didn't finish soon enough.")
	}
}
