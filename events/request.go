package events

import "net/http"

// Client used to fetch events. Usually *gobyairship.Client.
type Client interface {
	Post(url string, body interface{}) (*http.Response, error)
}

type DeviceType string

const (
	DeviceAmazon  DeviceType = "amazon"
	DeviceAndroid DeviceType = "android"
	DeviceIOS     DeviceType = "ios"
)

type Filter struct {
	Types        []Type              `json:"type,omitempty"`
	DeviceTypes  []DeviceType        `json:"device_types,omitempty"`
	Notification map[string]string   `json:"notification,omitempty"`
	Devices      []map[string]string `json:"devices,omitempty"`
}

type Start string

const (
	StartFirst = "EARLIEST"
	StartLast  = "LATEST"
)

type Request struct {
	Start   Start     `json:"start"`
	Offset  uint64    `json:"resume_offset"`
	Filters []*Filter `json:"filters,omitempty"`
}

// Fetch events using Client. Filters may be nil to fetch all events. If error
// is non-nil Response will stream events until Close is called.
func Fetch(c Client, s Start, offset uint64, filters []*Filter) (*Response, error) {
	req := &Request{Start: s, Offset: offset, Filters: filters}
	resp, err := c.Post("events", req)
	if err != nil {
		return nil, err
	}
	return newResponse(resp)
}
