package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	DefaultDiskSpillThreshold = 8 << 20
	jsonUTF8BOMLen            = 3
	// maxDecompressedBodySize limits the decompressed request body to 64 MB
	// to prevent decompression bomb attacks.
	maxDecompressedBodySize = 64 << 20
)

// PrereadBody wraps a fully buffered body so later consumers can reread it
// without another allocation or copy.
type PrereadBody struct {
	body   []byte
	reader *bytes.Reader
}

func NewPrereadBody(body []byte) *PrereadBody {
	return &PrereadBody{body: body, reader: bytes.NewReader(body)}
}

func (p *PrereadBody) Read(b []byte) (int, error) {
	if p == nil {
		return 0, io.EOF
	}
	return p.reader.Read(b)
}

func (p *PrereadBody) Close() error { return nil }

func (p *PrereadBody) Bytes() []byte {
	if p == nil {
		return nil
	}
	return p.body
}

// ReadRequestBodyWithDiskSpill keeps small bodies on the existing in-memory
// path and stores large, chunked, or compressed bodies in a read-only mmap.
func ReadRequestBodyWithDiskSpill(req *http.Request, threshold int64) ([]byte, func(), error) {
	if req == nil || req.Body == nil {
		return nil, nil, nil
	}

	encoding := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	compressed := encoding != "" && encoding != "identity"
	if threshold <= 0 || (!compressed && req.ContentLength >= 0 && req.ContentLength < threshold) {
		body, err := ReadRequestBodyWithPrealloc(req)
		return body, nil, err
	}

	reader := io.Reader(req.Body)
	closeReader := func() {}
	if compressed {
		var err error
		reader, closeReader, err = newDecompressedRequestBodyReader(encoding, req.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("decode Content-Encoding %q: %w", encoding, err)
		}
		reader = io.LimitReader(reader, maxDecompressedBodySize)
	}
	defer closeReader()

	mapped, err := NewMappedBody(func(w io.Writer) error {
		_, copyErr := io.Copy(w, reader)
		return copyErr
	})
	if err != nil {
		return nil, nil, err
	}
	body := mapped.Bytes()
	if compressed {
		req.Header.Del("Content-Encoding")
		req.Header.Del("Content-Length")
		req.ContentLength = int64(len(body))
	}
	return body, func() { _ = mapped.Close() }, nil
}

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based
// on content length, transparently decoding any Content-Encoding the upstream
// client used to compress the body (zstd, gzip, deflate).
// 已由 PrereadBody 回填的请求体直接返回其完整切片（零拷贝），不检查内部
// reader 是否已被消费——见 PrereadBody 的文档说明。
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}
	if preread, ok := req.Body.(*PrereadBody); ok {
		return preread.Bytes(), nil
	}

	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
		}
	}

	raw, err := readRequestBodyChunks(req.Body, capHint, req.ContentLength)
	if err != nil {
		return nil, err
	}

	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return raw, nil
	}

	decoded, err := decompressRequestBody(enc, raw)
	if err != nil {
		return nil, fmt.Errorf("decode Content-Encoding %q: %w", enc, err)
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded))

	return decoded, nil
}

// Read bounded chunks as bytes arrive, then assemble the exact-size result.
// This avoids doubling large buffers or eagerly allocating an untrusted
// Content-Length before the corresponding bytes have arrived.
func readRequestBodyChunks(reader io.Reader, initialCapacity int, contentLength int64) ([]byte, error) {
	capacity := initialCapacity
	var chunks [][]byte
	total := 0
	for {
		chunkCapacity := capacity
		if remaining := contentLength - int64(total); remaining >= 0 && remaining < int64(chunkCapacity) {
			chunkCapacity = int(remaining) + 1
		}
		chunk := make([]byte, chunkCapacity)
		n := 0
		var err error
		for n < len(chunk) && err == nil {
			var read int
			read, err = reader.Read(chunk[n:])
			n += read
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n > 0 {
			chunks = append(chunks, chunk[:n])
			total += n
		}
		if err != nil {
			if len(chunks) == 0 {
				return chunk[:0], nil
			}
			if len(chunks) == 1 {
				return chunks[0], nil
			}
			body := make([]byte, total)
			offset := 0
			for _, part := range chunks {
				offset += copy(body[offset:], part)
			}
			return body, nil
		}
		if capacity < requestBodyReadMaxInitCap {
			capacity *= 2
			if capacity > requestBodyReadMaxInitCap {
				capacity = requestBodyReadMaxInitCap
			}
		}
	}
}

// ReadLenientJSONRequestBodyWithPrealloc reads a request body and normalizes
// JSON string control bytes before strict validation.
func ReadLenientJSONRequestBodyWithPrealloc(req *http.Request, maxNormalizedBytes int64) ([]byte, error) {
	body, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		return nil, err
	}
	return NormalizeLenientJSONRequestBody(body, maxNormalizedBytes)
}

// ReadLenientJSONRequestBodyWithDiskSpill applies lenient JSON normalization
// after selecting the disk-backed large-body path.
func ReadLenientJSONRequestBodyWithDiskSpill(req *http.Request, threshold, maxNormalizedBytes int64) ([]byte, func(), error) {
	body, cleanup, err := ReadRequestBodyWithDiskSpill(req, threshold)
	if err != nil {
		return nil, nil, err
	}
	normalized, err := NormalizeLenientJSONRequestBody(body, maxNormalizedBytes)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, err
	}
	return normalized, cleanup, nil
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	reader, closeReader, err := newDecompressedRequestBodyReader(encoding, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer closeReader()
	return io.ReadAll(io.LimitReader(reader, maxDecompressedBodySize))
}

func newDecompressedRequestBodyReader(encoding string, raw io.Reader) (io.Reader, func(), error) {
	switch encoding {
	case "zstd":
		dec, err := zstd.NewReader(raw)
		if err != nil {
			return nil, nil, err
		}
		return dec, dec.Close, nil
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(raw)
		if err != nil {
			return nil, nil, err
		}
		return gr, func() { _ = gr.Close() }, nil
	case "deflate":
		zr, err := zlib.NewReader(raw)
		if err != nil {
			return nil, nil, err
		}
		return zr, func() { _ = zr.Close() }, nil
	default:
		return nil, nil, errors.New("unsupported Content-Encoding")
	}
}

// NormalizeLenientJSONRequestBody escapes raw control bytes that broken
// OpenAI-compatible clients sometimes place inside JSON strings.
func NormalizeLenientJSONRequestBody(body []byte, maxNormalizedBytes int64) ([]byte, error) {
	if maxNormalizedBytes <= 0 {
		maxNormalizedBytes = maxDecompressedBodySize
	}

	body = trimUTF8BOM(body)
	if len(body) == 0 {
		return body, nil
	}
	if int64(len(body)) > maxNormalizedBytes {
		return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
	}

	var out []byte
	inString := false
	escaped := false
	for i, b := range body {
		if inString && isJSONControlByte(b) {
			if out == nil {
				capHint := len(body) + 6
				if int64(capHint) > maxNormalizedBytes {
					capHint = int(maxNormalizedBytes)
				}
				out = make([]byte, 0, capHint)
				out = append(out, body[:i]...)
			}
			if int64(len(out)+6) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = appendJSONUnicodeEscape(out, b)
			escaped = false
			continue
		}

		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}

		if out != nil {
			if int64(len(out)+1) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = append(out, b)
		}
	}
	if out != nil {
		return out, nil
	}
	return body, nil
}

func trimUTF8BOM(body []byte) []byte {
	if len(body) >= jsonUTF8BOMLen && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		return body[jsonUTF8BOMLen:]
	}
	return body
}

func isJSONControlByte(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func appendJSONUnicodeEscape(dst []byte, b byte) []byte {
	const hex = "0123456789abcdef"
	return append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0x0f])
}
