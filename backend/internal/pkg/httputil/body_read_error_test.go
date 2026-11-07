package httputil

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"testing/iotest"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestReadRequestBodyWithPreallocReportsProgressOnReadError(t *testing.T) {
	reader := io.MultiReader(bytes.NewReader([]byte("partial")), iotest.ErrReader(io.ErrUnexpectedEOF))
	req := &http.Request{Body: io.NopCloser(reader), ContentLength: 100, Header: make(http.Header)}

	got, err := ReadRequestBodyWithPrealloc(req)

	var readErr *RequestBodyReadError
	if got != nil || !errors.As(err, &readErr) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read error lost its cause or progress: body=%q err=%v", got, err)
	}
	if readErr.BytesRead != int64(len("partial")) || readErr.Elapsed < 0 {
		t.Fatalf("unexpected progress: bytes=%d elapsed=%s", readErr.BytesRead, readErr.Elapsed)
	}
}

func TestDiskSpillRequestBodyReadFailuresReportRawProgress(t *testing.T) {
	var compressed bytes.Buffer
	encoder := gzip.NewWriter(&compressed)
	if _, err := encoder.Write(bytes.Repeat([]byte{'x'}, 64<<10)); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	deadlineErr := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
	for _, tc := range []struct {
		name          string
		payload       []byte
		encoding      string
		readErr       error
		wantErr       error
		cancelContext bool
		wantKind      string
		wantStatus    int
	}{
		{name: "raw timeout", payload: []byte("partial"), readErr: deadlineErr, wantErr: deadlineErr, wantKind: RequestBodyReadErrorTimeout, wantStatus: http.StatusRequestTimeout},
		{name: "raw timeout with canceled context", payload: []byte("partial"), readErr: deadlineErr, wantErr: deadlineErr, cancelContext: true, wantKind: RequestBodyReadErrorTimeout, wantStatus: http.StatusRequestTimeout},
		{name: "gzip header timeout", payload: []byte{0x1f, 0x8b, 0x08}, encoding: "gzip", readErr: deadlineErr, wantErr: deadlineErr, wantKind: RequestBodyReadErrorTimeout, wantStatus: http.StatusRequestTimeout},
		{name: "gzip header disconnect", payload: []byte{0x1f, 0x8b, 0x08}, encoding: "gzip", readErr: context.Canceled, wantErr: context.Canceled, wantKind: RequestBodyReadErrorClientDisconnect, wantStatus: StatusClientClosedRequest},
		{name: "gzip payload timeout counts compressed bytes", payload: compressed.Bytes(), encoding: "gzip", readErr: deadlineErr, wantErr: deadlineErr, wantKind: RequestBodyReadErrorTimeout, wantStatus: http.StatusRequestTimeout},
		{name: "invalid gzip", payload: []byte("invalid gzip header"), encoding: "gzip", readErr: io.EOF, wantErr: gzip.ErrHeader, wantKind: RequestBodyReadErrorDecodeContentEncoding, wantStatus: http.StatusBadRequest},
		{name: "raw truncated", payload: []byte("partial"), readErr: io.ErrUnexpectedEOF, wantErr: io.ErrUnexpectedEOF, wantKind: RequestBodyReadErrorTruncated, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := io.MultiReader(bytes.NewReader(tc.payload), requestBodyBudgetReaderFunc(func([]byte) (int, error) {
				if tc.cancelContext {
					cancel()
				}
				return 0, tc.readErr
			}))
			req := &http.Request{Body: io.NopCloser(reader), ContentLength: -1, Header: make(http.Header)}
			req = req.WithContext(ctx)
			req.Header.Set("Content-Encoding", tc.encoding)

			body, cleanup, err := ReadRequestBodyWithDiskSpill(req, 1)
			if cleanup != nil {
				defer cleanup()
			}
			var readErr *RequestBodyReadError
			if body != nil || cleanup != nil || !errors.Is(err, tc.wantErr) || !errors.As(err, &readErr) {
				t.Fatalf("want wrapped %v without a body, got len=%d cleanup=%t err=%v", tc.wantErr, len(body), cleanup != nil, err)
			}
			if readErr.BytesRead != int64(len(tc.payload)) || readErr.Elapsed <= 0 {
				t.Fatalf("raw progress: bytes=%d elapsed=%s, want bytes=%d", readErr.BytesRead, readErr.Elapsed, len(tc.payload))
			}
			failure := DescribeRequestBodyReadFailure(req, err)
			if failure.Kind != tc.wantKind || failure.Status != tc.wantStatus {
				t.Fatalf("got %s/%d, want %s/%d", failure.Kind, failure.Status, tc.wantKind, tc.wantStatus)
			}
		})
	}
}

func TestDescribeRequestBodyReadFailure(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	deadlineErr := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
	resetErr := &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}

	tests := []struct {
		name       string
		ctx        context.Context
		err        error
		wantKind   string
		wantStatus int
	}{
		// net/http cancels the request context on an HTTP/1 read timeout too;
		// the timeout must still win over the disconnect classification.
		{"http1 read deadline", canceledCtx, &RequestBodyReadError{Err: deadlineErr}, RequestBodyReadErrorTimeout, http.StatusRequestTimeout},
		{"http2 read deadline", context.Background(), fmt.Errorf("%w", os.ErrDeadlineExceeded), RequestBodyReadErrorTimeout, http.StatusRequestTimeout},
		{"compressed timeout", canceledCtx, fmt.Errorf("decode Content-Encoding gzip: %w", deadlineErr), RequestBodyReadErrorTimeout, http.StatusRequestTimeout},
		{"compressed disconnect", context.Background(), fmt.Errorf("decode Content-Encoding gzip: %w", context.Canceled), RequestBodyReadErrorClientDisconnect, StatusClientClosedRequest},
		{"http1 disconnect mid body", canceledCtx, &RequestBodyReadError{Err: io.ErrUnexpectedEOF}, RequestBodyReadErrorClientDisconnect, StatusClientClosedRequest},
		{"connection reset", context.Background(), resetErr, RequestBodyReadErrorClientDisconnect, StatusClientClosedRequest},
		{"http2 client disconnected", context.Background(), errors.New("client disconnected"), RequestBodyReadErrorClientDisconnect, StatusClientClosedRequest},
		{"truncated body", context.Background(), &RequestBodyReadError{Err: io.ErrUnexpectedEOF}, RequestBodyReadErrorTruncated, http.StatusBadRequest},
		{"http2 short body", context.Background(), errors.New("request declared a Content-Length of 10 but only wrote 3 bytes"), RequestBodyReadErrorTruncated, http.StatusBadRequest},
		{"max bytes", context.Background(), &RequestBodyReadError{Err: &http.MaxBytesError{Limit: 64}}, RequestBodyReadErrorMaxBytes, http.StatusRequestEntityTooLarge},
		{"decode", context.Background(), errors.New(`decode Content-Encoding "gzip": unexpected EOF`), RequestBodyReadErrorDecodeContentEncoding, http.StatusBadRequest},
		{"unsupported encoding", context.Background(), errors.New(`decode Content-Encoding "br": unsupported Content-Encoding`), RequestBodyReadErrorUnsupportedContentEncoding, http.StatusBadRequest},
		{"other", context.Background(), errors.New("boom"), RequestBodyReadErrorIO, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(tc.ctx, http.MethodPost, "/v1/messages", nil)
			if err != nil {
				t.Fatal(err)
			}
			got := DescribeRequestBodyReadFailure(req, tc.err)
			if got.Kind != tc.wantKind || got.Status != tc.wantStatus || got.Message == "" {
				t.Fatalf("got kind=%q status=%d message=%q, want kind=%q status=%d", got.Kind, got.Status, got.Message, tc.wantKind, tc.wantStatus)
			}
		})
	}

	if got := DescribeRequestBodyReadFailure(nil, &http.MaxBytesError{Limit: 64}); got.MaxBytesLimit != 64 {
		t.Fatalf("max bytes limit not reported: %d", got.MaxBytesLimit)
	}
	if got := DescribeRequestBodyReadFailure(nil, errors.New("boom")); got.Message != "Failed to read request body" {
		t.Fatalf("generic failures must keep the stable message, got %q", got.Message)
	}
}

func TestRequestBodyReadFailureLogFieldsArePayloadFree(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("secret-payload-marker"))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = 4096
	req.Header.Set("Content-Encoding", "zstd")
	readErr := &RequestBodyReadError{
		Err:       &net.OpError{Op: "read", Net: "tcp", Err: errors.New("10.0.0.1 secret-payload-marker")},
		BytesRead: 1024,
		Elapsed:   1500 * time.Millisecond,
	}

	core, logs := observer.New(zap.WarnLevel)
	zap.New(core).Warn("read request body failed", DescribeRequestBodyReadFailure(req, readErr).LogFields()...)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	want := map[string]any{
		"error_kind":       RequestBodyReadErrorTransport,
		"status":           int64(http.StatusBadRequest),
		"content_encoding": "zstd",
		"content_length":   int64(4096),
		"bytes_read":       int64(1024),
		"read_ms":          int64(1500),
		"chunked":          false,
		"protocol":         "HTTP/1.1",
	}
	for key, value := range want {
		if fields[key] != value {
			t.Fatalf("field %s = %#v, want %#v (all: %v)", key, fields[key], value, fields)
		}
	}
	if strings.Contains(fmt.Sprint(fields), "secret-payload-marker") || strings.Contains(fmt.Sprint(fields), "10.0.0.1") {
		t.Fatalf("log fields leaked error text: %v", fields)
	}
}
