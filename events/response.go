package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// WrongType is returned by per-Type methods on Event if method called doesn't
// match the Event's type.
var WrongType = errors.New("wrong type for event")

// Type of Event. Events contain per-Type methods to properly decode the Event
// body into a the desired Type.
type Type string

const (
	TypePush      Type = "PUSH"
	TypeOpen      Type = "OPEN"
	TypeSend      Type = "SEND"
	TypeClose     Type = "CLOSE"
	TypeTagChange Type = "TAG_CHANGE"
	TypeUninstall Type = "UNINSTALL"
	TypeFirst     Type = "FIRST_OPEN"
	TypeCustom    Type = "CUSTOM"
	TypeLocation  Type = "LOCATION"
)

type Event struct {
	ID        string          `json:"id"`
	Type      Type            `json:"type"`
	Occurred  time.Time       `json:"occurred"`
	Processed time.Time       `json:"processed"`
	Offset    uint64          `json:"offset,string"`
	Body      json.RawMessage `json:"body"`
	Devices   struct {
		Amazon    string `json:"amazon_channel"`
		Android   string `json:"android_channel"`
		IOS       string `json:"ios_channel"`
		NamedUser string `json:"named_user_id"`
	} `json:"devices"`
}

type Push struct {
	// PushID is the unique identifier for the push, included in responses to the
	// push API.
	PushID string `json:"push_id"`

	// GroupID is an optional identifier of the group this push is associated
	// with; group IDs are created by both automation and push to local time.
	GroupID string `json:"group_id"`

	// Payload is the specification of the push as sent via the API.
	Payload []byte `json:"payload"`
}

func (e *Event) Push() (*Push, error) {
	if e.Type != TypePush {
		return nil, WrongType
	}
	p := Push{}
	if err := json.Unmarshal(e.Body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

type Open struct {
	// LastDelivered is the push ID of the last notification Urban Airship
	// attempted delivered to this device.
	LastDelivered string `json:"last_delivered"`

	// SessionID is an identifier for the "session" of user activity. This can be
	// missing for some kinds of background activity.
	SessionID string `json:"session_id"`

	// ConvertingPush is marked as TBD in the spec. If the user opened the app
	// based on interacting with a push notification, this entry will include the
	// push ID.
	ConvertingPush string `json:"converting_push"`
}

func (e *Event) Open() (*Open, error) {
	if e.Type != TypeOpen {
		return nil, WrongType
	}
	o := Open{}
	if err := json.Unmarshal(e.Body, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// Send events are emitted for each device identified by the audience selection
// of a push. device will be present in the event to specify which channel
// received the push.
type Send struct {
	PushID string `json:"push_id"`
}

func (e *Event) Send() (*Send, error) {
	if e.Type != TypeSend {
		return nil, WrongType
	}
	s := Send{}
	if err := json.Unmarshal(e.Body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Close events are emitted Each time a user closes the application. Note that
// close events are often latent, as they may not be delivered over the network
// until much later.
type Close struct {
	SessionID string `json:"session_id"`
}

func (e *Event) Close() (*Close, error) {
	if e.Type != TypeClose {
		return nil, WrongType
	}
	c := Close{}
	if err := json.Unmarshal(e.Body, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// TagChange events are emitted Each time a tag is added or removed from a
// channel.
type TagChange struct {
	// Add maps tag groups to tags. The set of tag group/tag pairs in this object
	// define the tags added to the device.
	Add map[string][]string `json:"add"`

	// Remove maps tag groups to tags. The set of tag group/tag pairs in this
	// object define the tags removed from the device.
	Remove map[string][]string `json:"remove"`

	// Current maps tag groups to tags. The set of tag group/tag pairs in this
	// object define the current state of the device AFTER the mutation has taken
	// effect.
	Current map[string][]string `json:"current"`
}

func (e *Event) TagChange() (*TagChange, error) {
	if e.Type != TypeTagChange {
		return nil, WrongType
	}
	t := TagChange{}
	if err := json.Unmarshal(e.Body, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Location events include the latitude and longitude of the device.
type Location struct {
	Lat json.Number `json:"latitude"`
	Lon json.Number `json:"longitude"`

	// Foreground indicates whether the application was foregrounded when the
	// event fired.
	Foreground bool `json:"foreground"`
}

func (e *Event) Location() (*Location, error) {
	if e.Type != TypeLocation {
		return nil, WrongType
	}
	loc := Location{}
	if err := json.Unmarshal(e.Body, &loc); err != nil {
		return nil, err
	}
	return &loc, nil
}

type Response struct {
	out  chan *Event
	body io.ReadCloser

	mu     *sync.Mutex
	closed chan struct{}
	err    error
}

func newResponse(resp *http.Response) (*Response, error) {
	if resp.StatusCode != 200 {
		//TODO Prettier error var?
		return nil, fmt.Errorf("non-200 response: %d", resp.StatusCode)
	}
	r := &Response{
		out:  make(chan *Event, 10), // provide some buffering
		body: resp.Body,
		mu:   new(sync.Mutex),
	}
	go func() {
		dec := json.NewDecoder(r.body)
		for {
			var ev Event
			if err := dec.Decode(&ev); err != nil {
				r.mu.Lock()
				defer r.mu.Unlock()
				r.err = err
				close(r.out)
				return
			}
			select {
			case r.out <- &ev:
			case <-r.closed:
				return
			}
		}
	}()
	return r, nil
}

// Events returns a chan that emits Events until closed.
func (r *Response) Events() <-chan *Event { return r.out }

// Close the events stream. Safe to call concurrently.
func (r *Response) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.closed:
		return
	default:
		r.body.Close()
		close(r.closed)
	}
}

// Err which caused the event stream to end or nil. May be checked when the
// chan returned by Events() is closed. Safe for concurrent access.
func (r *Response) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}
