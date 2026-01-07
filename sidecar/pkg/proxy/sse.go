package proxy

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vitorbari/mcp-operator/sidecar/pkg/mcp"
)

// SSEContentType is the Content-Type header value for SSE responses.
const SSEContentType = "text/event-stream"

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	// Event is the event type (from "event:" field).
	Event string
	// Data is the event data (from "data:" field).
	Data string
	// ID is the event ID (from "id:" field).
	ID string
	// Retry is the reconnection time in milliseconds (from "retry:" field).
	Retry string
}

// SSEMetricsRecorder defines the interface for recording SSE metrics.
type SSEMetricsRecorder interface {
	SSEConnectionOpened(ctx context.Context)
	SSEConnectionClosed(ctx context.Context, duration time.Duration)
	SSEEventReceived(ctx context.Context, eventType string)
	RecordToolCall(ctx context.Context, toolName string)
	RecordResourceRead(ctx context.Context, resourceURI string)
	RecordError(ctx context.Context, method string, errorCode int)
}

// IsSSEResponse checks if the response is an SSE stream based on Content-Type.
func IsSSEResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, SSEContentType)
}

// IsSSEContentType checks if the content type indicates SSE.
func IsSSEContentType(contentType string) bool {
	return strings.HasPrefix(contentType, SSEContentType)
}

// SSEStreamCopier handles streaming SSE responses while collecting metrics.
type SSEStreamCopier struct {
	recorder  SSEMetricsRecorder
	startTime time.Time
}

// NewSSEStreamCopier creates a new SSEStreamCopier.
func NewSSEStreamCopier(recorder SSEMetricsRecorder) *SSEStreamCopier {
	return &SSEStreamCopier{
		recorder:  recorder,
		startTime: time.Now(),
	}
}

// StreamResponse streams an SSE response to the client while parsing events for metrics.
// It copies response headers, streams body line-by-line, and flushes after each event.
func (s *SSEStreamCopier) StreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) (int64, error) {
	// Record connection opened
	if s.recorder != nil {
		s.recorder.SSEConnectionOpened(ctx)
		defer func() {
			s.recorder.SSEConnectionClosed(ctx, time.Since(s.startTime))
		}()
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Get the flusher for streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		// If flusher not available, fall back to regular copy
		return io.Copy(w, resp.Body)
	}

	// Stream the response line by line
	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer size for potentially large data fields
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var bytesWritten int64
	var currentEvent SSEEvent

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return bytesWritten, ctx.Err()
		default:
		}

		line := scanner.Text()

		// Write the line to the response
		n, err := w.Write([]byte(line + "\n"))
		bytesWritten += int64(n)
		if err != nil {
			return bytesWritten, err
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "event:") {
			currentEvent.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			// Append data (SSE spec allows multiple data lines)
			data := strings.TrimPrefix(line, "data:")
			if currentEvent.Data != "" {
				currentEvent.Data += "\n" + data
			} else {
				currentEvent.Data = data
			}
		} else if strings.HasPrefix(line, "id:") {
			currentEvent.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "retry:") {
			currentEvent.Retry = strings.TrimSpace(strings.TrimPrefix(line, "retry:"))
		} else if line == "" {
			// Empty line marks end of event
			if currentEvent.Event != "" || currentEvent.Data != "" {
				s.processEvent(ctx, &currentEvent)
				currentEvent = SSEEvent{} // Reset for next event
			}
			// Flush after each complete event
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		return bytesWritten, err
	}

	return bytesWritten, nil
}

// processEvent processes a complete SSE event and records metrics.
func (s *SSEStreamCopier) processEvent(ctx context.Context, event *SSEEvent) {
	if s.recorder == nil {
		return
	}

	// Record event type metric
	eventType := event.Event
	if eventType == "" {
		eventType = "message" // Default SSE event type
	}
	s.recorder.SSEEventReceived(ctx, eventType)

	// Try to parse MCP data from the event
	if event.Data != "" {
		s.parseMCPData(ctx, strings.TrimSpace(event.Data))
	}
}

// parseMCPData attempts to parse MCP JSON-RPC data from SSE event data.
func (s *SSEStreamCopier) parseMCPData(ctx context.Context, data string) {
	// Try parsing as request (for notifications/requests sent via SSE)
	if req, err := mcp.ParseRequest([]byte(data)); err == nil {
		if req.Method == mcp.MethodToolsCall && req.ToolName != "" {
			s.recorder.RecordToolCall(ctx, req.ToolName)
		}
		if req.Method == mcp.MethodResourcesRead && req.ResourceURI != "" {
			s.recorder.RecordResourceRead(ctx, req.ResourceURI)
		}
		return
	}

	// Try parsing as response (for responses sent via SSE)
	if resp, err := mcp.ParseResponse([]byte(data)); err == nil {
		if resp.IsError {
			s.recorder.RecordError(ctx, "sse", resp.ErrorCode)
		}
	}
}

// ParseSSEEvent parses a single SSE event from accumulated lines.
// Returns the parsed event and true if the line contributes to an event,
// or nil and false if it's an empty line (event boundary).
func ParseSSEEvent(line string) (*SSEEvent, bool) {
	line = strings.TrimSpace(line)

	// Empty line indicates event boundary
	if line == "" {
		return nil, false
	}

	// Comment lines start with colon
	if strings.HasPrefix(line, ":") {
		return nil, true // Ignore comments but continue parsing
	}

	event := &SSEEvent{}

	if strings.HasPrefix(line, "event:") {
		event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		return event, true
	}
	if strings.HasPrefix(line, "data:") {
		event.Data = strings.TrimPrefix(line, "data:")
		return event, true
	}
	if strings.HasPrefix(line, "id:") {
		event.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		return event, true
	}
	if strings.HasPrefix(line, "retry:") {
		event.Retry = strings.TrimSpace(strings.TrimPrefix(line, "retry:"))
		return event, true
	}

	// Unknown field, ignore per SSE spec
	return nil, true
}
