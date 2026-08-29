package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

const samplePayload = `{"model":"gpt-5.5","input":"hi","stream":false}`

var benchmarkRequestBodySink []byte
var benchmarkRequestBodyLenSink int

func newRequestWithBody(t *testing.T, body []byte, encoding string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	req.ContentLength = int64(len(body))
	return req
}

func TestReadRequestBodyWithPrealloc_PassesThroughIdentity(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesZstd(t *testing.T) {
	enc, _ := zstd.NewWriter(nil)
	compressed := enc.EncodeAll([]byte(samplePayload), nil)
	_ = enc.Close()

	req := newRequestWithBody(t, compressed, "zstd")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
	if req.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be cleared after decoding")
	}
	if req.ContentLength != int64(len(samplePayload)) {
		t.Fatalf("ContentLength not updated: %d", req.ContentLength)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesGzip(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "gzip")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesDeflate(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "deflate")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsUnsupportedEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "br")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for unsupported encoding, got nil")
	}
	if !strings.Contains(err.Error(), "br") {
		t.Fatalf("error should mention encoding, got %v", err)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsCorruptZstd(t *testing.T) {
	req := newRequestWithBody(t, []byte("not actually zstd"), "zstd")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for corrupt zstd body, got nil")
	}
}

func TestReadRequestBodyWithPrealloc_NilBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil body, got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RespectsIdentityEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "identity")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_UsesKnownLargeContentLength(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 3<<20+123)
	req := newRequestWithBody(t, payload, "")

	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap(got) != len(payload) {
		t.Fatalf("expected exact preallocation for known body: len=%d cap=%d", len(payload), cap(got))
	}
}

func TestReadRequestBodyWithDiskSpill_LargeIdentityBody(t *testing.T) {
	payload := bytes.Repeat([]byte("disk-backed-body"), 4096)
	req := newRequestWithBody(t, payload, "")

	got, cleanup, err := ReadRequestBodyWithDiskSpill(req, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected disk-backed cleanup")
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("disk-backed body mismatch")
	}
	cleanup()
	cleanup()
}

func TestReadRequestBodyWithDiskSpill_CompressedBody(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	payload := bytes.Repeat([]byte(samplePayload), 2048)
	if _, err := zw.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	req := newRequestWithBody(t, compressed.Bytes(), "gzip")

	got, cleanup, err := ReadRequestBodyWithDiskSpill(req, 1<<20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	if !bytes.Equal(got, payload) {
		t.Fatal("decompressed disk-backed body mismatch")
	}
	if req.Header.Get("Content-Encoding") != "" {
		t.Fatal("Content-Encoding should be cleared")
	}
	if req.ContentLength != int64(len(payload)) {
		t.Fatalf("ContentLength not updated: %d", req.ContentLength)
	}
}

func TestMappedBody_ReaderKeepsMappingAliveAfterOwnerClose(t *testing.T) {
	mapped, err := NewMappedBody(func(w io.Writer) error {
		_, writeErr := io.WriteString(w, samplePayload)
		return writeErr
	})
	if err != nil {
		t.Fatalf("create mapped body: %v", err)
	}
	reader := mapped.NewReader()
	if err := mapped.Close(); err != nil {
		t.Fatalf("close owner: %v", err)
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read retained mapping: %v", err)
	}
	if !bytes.Equal(got, []byte(samplePayload)) {
		t.Fatalf("retained body mismatch: got %q", got)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
}

func BenchmarkReadRequestBodyWithPrealloc_IncidentSize(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 38_707_514)

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
		if err != nil {
			b.Fatal(err)
		}
		req.Body = io.NopCloser(io.LimitReader(bytes.NewReader(payload), int64(len(payload))))
		req.ContentLength = int64(len(payload))
		benchmarkRequestBodySink, err = ReadRequestBodyWithPrealloc(req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadRequestBodyWithDiskSpill_IncidentSize(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 38_707_514)

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
		if err != nil {
			b.Fatal(err)
		}
		req.Body = io.NopCloser(io.LimitReader(bytes.NewReader(payload), int64(len(payload))))
		req.ContentLength = int64(len(payload))
		body, cleanup, err := ReadRequestBodyWithDiskSpill(req, DefaultDiskSpillThreshold)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkRequestBodyLenSink = len(body)
		cleanup()
	}
}
