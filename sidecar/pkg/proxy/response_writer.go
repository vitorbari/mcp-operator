package proxy

import (
	"bytes"
	"net/http"
)

const (
	// DefaultMaxBodyCapture is the default maximum size for captured response bodies (1MB).
	DefaultMaxBodyCapture = 1024 * 1024
)

// responseCapture wraps http.ResponseWriter to capture the response body and status code.
type responseCapture struct {
	http.ResponseWriter
	statusCode   int
	body         bytes.Buffer
	maxCapture   int
	bytesWritten int64
}

// newResponseCapture creates a new responseCapture with the given max capture size.
// If maxCapture is 0, DefaultMaxBodyCapture is used.
func newResponseCapture(w http.ResponseWriter, maxCapture int) *responseCapture {
	if maxCapture <= 0 {
		maxCapture = DefaultMaxBodyCapture
	}
	return &responseCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		maxCapture:     maxCapture,
	}
}

// WriteHeader captures the status code and writes it to the underlying ResponseWriter.
func (rc *responseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.ResponseWriter.WriteHeader(code)
}

// Write captures the response body (up to maxCapture bytes) and writes to the underlying ResponseWriter.
func (rc *responseCapture) Write(b []byte) (int, error) {
	// Capture body up to the limit
	if rc.body.Len() < rc.maxCapture {
		remaining := rc.maxCapture - rc.body.Len()
		if len(b) <= remaining {
			rc.body.Write(b)
		} else {
			rc.body.Write(b[:remaining])
		}
	}

	n, err := rc.ResponseWriter.Write(b)
	rc.bytesWritten += int64(n)
	return n, err
}

// Flush implements http.Flusher for SSE compatibility.
func (rc *responseCapture) Flush() {
	if flusher, ok := rc.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// StatusCode returns the captured HTTP status code.
func (rc *responseCapture) StatusCode() int {
	return rc.statusCode
}

// Body returns the captured response body.
func (rc *responseCapture) Body() []byte {
	return rc.body.Bytes()
}

// BytesWritten returns the total number of bytes written to the response.
func (rc *responseCapture) BytesWritten() int64 {
	return rc.bytesWritten
}
