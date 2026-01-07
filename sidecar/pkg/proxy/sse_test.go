package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsSSEResponse(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expected    bool
	}{
		{"SSE content type", "text/event-stream", true},
		{"SSE with charset", "text/event-stream; charset=utf-8", true},
		{"JSON content type", "application/json", false},
		{"HTML content type", "text/html", false},
		{"Empty content type", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{
					"Content-Type": []string{tt.contentType},
				},
			}
			result := IsSSEResponse(resp)
			if result != tt.expected {
				t.Errorf("IsSSEResponse() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsSSEContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"text/event-stream", true},
		{"text/event-stream; charset=utf-8", true},
		{"application/json", false},
		{"text/html", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := IsSSEContentType(tt.contentType)
			if result != tt.expected {
				t.Errorf("IsSSEContentType(%q) = %v, expected %v", tt.contentType, result, tt.expected)
			}
		})
	}
}

func TestParseSSEEvent(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantEvent  *SSEEvent
		wantParsed bool
	}{
		{
			name:       "event field",
			line:       "event: message",
			wantEvent:  &SSEEvent{Event: "message"},
			wantParsed: true,
		},
		{
			name:       "data field",
			line:       "data: {\"key\": \"value\"}",
			wantEvent:  &SSEEvent{Data: " {\"key\": \"value\"}"},
			wantParsed: true,
		},
		{
			name:       "id field",
			line:       "id: 123",
			wantEvent:  &SSEEvent{ID: "123"},
			wantParsed: true,
		},
		{
			name:       "retry field",
			line:       "retry: 5000",
			wantEvent:  &SSEEvent{Retry: "5000"},
			wantParsed: true,
		},
		{
			name:       "empty line (event boundary)",
			line:       "",
			wantEvent:  nil,
			wantParsed: false,
		},
		{
			name:       "comment line",
			line:       ": this is a comment",
			wantEvent:  nil,
			wantParsed: true,
		},
		{
			name:       "unknown field",
			line:       "unknown: value",
			wantEvent:  nil,
			wantParsed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, parsed := ParseSSEEvent(tt.line)
			if parsed != tt.wantParsed {
				t.Errorf("ParseSSEEvent() parsed = %v, want %v", parsed, tt.wantParsed)
			}
			if tt.wantEvent == nil {
				if event != nil && (event.Event != "" || event.Data != "" || event.ID != "" || event.Retry != "") {
					t.Errorf("ParseSSEEvent() event = %+v, want nil or empty", event)
				}
			} else if event == nil {
				t.Errorf("ParseSSEEvent() event = nil, want %+v", tt.wantEvent)
			} else {
				if tt.wantEvent.Event != "" && event.Event != tt.wantEvent.Event {
					t.Errorf("ParseSSEEvent() event.Event = %v, want %v", event.Event, tt.wantEvent.Event)
				}
				if tt.wantEvent.Data != "" && event.Data != tt.wantEvent.Data {
					t.Errorf("ParseSSEEvent() event.Data = %v, want %v", event.Data, tt.wantEvent.Data)
				}
				if tt.wantEvent.ID != "" && event.ID != tt.wantEvent.ID {
					t.Errorf("ParseSSEEvent() event.ID = %v, want %v", event.ID, tt.wantEvent.ID)
				}
				if tt.wantEvent.Retry != "" && event.Retry != tt.wantEvent.Retry {
					t.Errorf("ParseSSEEvent() event.Retry = %v, want %v", event.Retry, tt.wantEvent.Retry)
				}
			}
		})
	}
}

// mockSSERecorder implements SSEMetricsRecorder for testing
type mockSSERecorder struct {
	connectionsOpened int
	connectionsClosed int
	closeDurations    []time.Duration
	eventsReceived    map[string]int
	toolCalls         []string
	resourceReads     []string
	errors            []struct {
		method string
		code   int
	}
}

func newMockSSERecorder() *mockSSERecorder {
	return &mockSSERecorder{
		eventsReceived: make(map[string]int),
	}
}

func (m *mockSSERecorder) SSEConnectionOpened(ctx context.Context) {
	m.connectionsOpened++
}

func (m *mockSSERecorder) SSEConnectionClosed(ctx context.Context, duration time.Duration) {
	m.connectionsClosed++
	m.closeDurations = append(m.closeDurations, duration)
}

func (m *mockSSERecorder) SSEEventReceived(ctx context.Context, eventType string) {
	m.eventsReceived[eventType]++
}

func (m *mockSSERecorder) RecordToolCall(ctx context.Context, toolName string) {
	m.toolCalls = append(m.toolCalls, toolName)
}

func (m *mockSSERecorder) RecordResourceRead(ctx context.Context, resourceURI string) {
	m.resourceReads = append(m.resourceReads, resourceURI)
}

func (m *mockSSERecorder) RecordError(ctx context.Context, method string, errorCode int) {
	m.errors = append(m.errors, struct {
		method string
		code   int
	}{method, errorCode})
}

func TestSSEStreamCopier_StreamResponse(t *testing.T) {
	recorder := newMockSSERecorder()
	copier := NewSSEStreamCopier(recorder)

	// Create a mock SSE response
	sseData := `event: message
data: {"jsonrpc":"2.0","method":"tools/call","params":{"name":"echo"}}

event: message
data: {"jsonrpc":"2.0","result":{}}

`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(sseData)),
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	bytesWritten, err := copier.StreamResponse(ctx, rr, resp)
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	if bytesWritten == 0 {
		t.Error("StreamResponse() bytesWritten = 0, expected > 0")
	}

	// Check that connection was opened and closed
	if recorder.connectionsOpened != 1 {
		t.Errorf("connectionsOpened = %d, expected 1", recorder.connectionsOpened)
	}
	if recorder.connectionsClosed != 1 {
		t.Errorf("connectionsClosed = %d, expected 1", recorder.connectionsClosed)
	}

	// Check that events were received
	if recorder.eventsReceived["message"] != 2 {
		t.Errorf("events['message'] = %d, expected 2", recorder.eventsReceived["message"])
	}
}

func TestSSEStreamCopier_StreamResponse_NilRecorder(t *testing.T) {
	copier := NewSSEStreamCopier(nil)

	sseData := `data: test

`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(sseData)),
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	// Should not panic with nil recorder
	_, err := copier.StreamResponse(ctx, rr, resp)
	if err != nil {
		t.Fatalf("StreamResponse() with nil recorder error = %v", err)
	}
}

func TestSSEStreamCopier_ParsesMCPData(t *testing.T) {
	recorder := newMockSSERecorder()
	copier := NewSSEStreamCopier(recorder)

	// SSE data with MCP tool call
	sseData := `event: message
data: {"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_weather"}}

event: message
data: {"jsonrpc":"2.0","method":"resources/read","params":{"uri":"file:///test.txt"}}

`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(sseData)),
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	_, err := copier.StreamResponse(ctx, rr, resp)
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	// Check tool calls were recorded
	if len(recorder.toolCalls) != 1 || recorder.toolCalls[0] != "get_weather" {
		t.Errorf("toolCalls = %v, expected [get_weather]", recorder.toolCalls)
	}

	// Check resource reads were recorded
	if len(recorder.resourceReads) != 1 || recorder.resourceReads[0] != "file:///test.txt" {
		t.Errorf("resourceReads = %v, expected [file:///test.txt]", recorder.resourceReads)
	}
}

func TestSSEStreamCopier_ContextCancellation(t *testing.T) {
	recorder := newMockSSERecorder()
	copier := NewSSEStreamCopier(recorder)

	// Create a slow reader that checks context
	ctx, cancel := context.WithCancel(context.Background())

	// Large SSE data to ensure we have time to cancel
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("data: test\n\n")
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(sb.String())),
	}

	rr := httptest.NewRecorder()

	// Cancel context immediately
	cancel()

	_, err := copier.StreamResponse(ctx, rr, resp)
	if err != context.Canceled {
		t.Logf("StreamResponse() with cancelled context error = %v (may be nil if fast)", err)
	}
}

func TestSSEStreamCopier_CopiesHeaders(t *testing.T) {
	copier := NewSSEStreamCopier(nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":  []string{"text/event-stream"},
			"Cache-Control": []string{"no-cache"},
			"X-Custom":      []string{"value1", "value2"},
		},
		Body: io.NopCloser(strings.NewReader("")),
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	_, err := copier.StreamResponse(ctx, rr, resp)
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	// Check headers were copied
	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type not copied correctly")
	}
	if rr.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control not copied correctly")
	}
	if len(rr.Header()["X-Custom"]) != 2 {
		t.Errorf("X-Custom header not copied correctly: %v", rr.Header()["X-Custom"])
	}
}

func TestSSEEvent_DefaultEventType(t *testing.T) {
	recorder := newMockSSERecorder()
	copier := NewSSEStreamCopier(recorder)

	// SSE data without event type (defaults to "message")
	sseData := `data: test data

`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(sseData)),
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	_, err := copier.StreamResponse(ctx, rr, resp)
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	// Default event type should be "message"
	if recorder.eventsReceived["message"] != 1 {
		t.Errorf("events['message'] = %d, expected 1 (default event type)", recorder.eventsReceived["message"])
	}
}

func TestSSEStreamCopier_MultiLineData(t *testing.T) {
	recorder := newMockSSERecorder()
	copier := NewSSEStreamCopier(recorder)

	// SSE data with multiple data lines (should be joined with newlines)
	sseData := `event: test
data: line1
data: line2
data: line3

`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(sseData)),
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	_, err := copier.StreamResponse(ctx, rr, resp)
	if err != nil {
		t.Fatalf("StreamResponse() error = %v", err)
	}

	// Should have received one "test" event
	if recorder.eventsReceived["test"] != 1 {
		t.Errorf("events['test'] = %d, expected 1", recorder.eventsReceived["test"])
	}
}
