package events_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/lytics/gobyairship/events"
)

type memClient struct {
	body io.ReadCloser
}

func (c *memClient) Post(url string, body interface{}, extra http.Header) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       c.body,
	}, nil
}

func BenchmarkCloseEvents(b *testing.B) {
	// Create 50 MB worth of data
	const line = `{"id":"4e175876-2ac1-665f-57c5-2f714a45601b","type":"CLOSE","offset":"0","occurred":"2015-05-27T11:32:07.729Z","processed":"2015-05-27T11:32:07.729Z","device":{"ios_channel":"af545191-d7b1-4b6d-8d33-6cfc4915edf0"},"body":{"session_id":"30f738bd-ecce-9f2b-536b-63e8d5e26aca"}}` + "\n"
	data := bytes.Repeat([]byte(line), (50*1024*1024)/len(line))
	total := int64(len(data))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c := &memClient{body: ioutil.NopCloser(bytes.NewReader(data))}
		b.StartTimer()

		resp, err := events.Fetch(c, events.StartOffset, 0, nil)
		if err != nil {
			b.Fatal(err)
		}

		for ev := range resp.Events() {
			if ev.Type != events.TypeClose {
				b.Fatalf("Unexpected type: %s", ev.Type)
			}
			cls, err := ev.Close()
			if err != nil {
				b.Fatal(err)
			}
			if cls.SessionID != "30f738bd-ecce-9f2b-536b-63e8d5e26aca" {
				b.Fatalf("Unexpected session ID: %s", cls.SessionID)
			}
		}
		b.SetBytes(total)
	}
}
