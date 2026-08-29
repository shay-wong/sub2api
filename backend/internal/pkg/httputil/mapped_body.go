package httputil

import (
	"bytes"
	"io"
	"sync"
)

// MappedBody owns disk-backed bytes until the owner and all readers close.
type MappedBody struct {
	data      []byte
	releaseFn func() error
	ownerOnce sync.Once
	mu        sync.Mutex
	refs      int
	err       error
}

func newMappedBody(data []byte, releaseFn func() error) *MappedBody {
	return &MappedBody{data: data, releaseFn: releaseFn, refs: 1}
}

// Bytes returns the mapped bytes while the owner remains open.
func (b *MappedBody) Bytes() []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data
}

// NewReader retains the mapping until the reader reaches EOF or is closed.
func (b *MappedBody) NewReader() io.ReadCloser {
	if b == nil {
		return io.NopCloser(bytes.NewReader(nil))
	}
	b.mu.Lock()
	if b.refs == 0 {
		b.mu.Unlock()
		return io.NopCloser(bytes.NewReader(nil))
	}
	b.refs++
	reader := bytes.NewReader(b.data)
	b.mu.Unlock()
	return &mappedBodyReader{Reader: reader, release: b.release}
}

// Close releases the owner's reference. Active readers keep the mapping alive.
func (b *MappedBody) Close() error {
	if b == nil {
		return nil
	}
	b.ownerOnce.Do(b.release)
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *MappedBody) release() {
	b.mu.Lock()
	if b.refs == 0 {
		b.mu.Unlock()
		return
	}
	b.refs--
	if b.refs > 0 {
		b.mu.Unlock()
		return
	}
	b.data = nil
	releaseFn := b.releaseFn
	b.mu.Unlock()

	if releaseFn != nil {
		err := releaseFn()
		b.mu.Lock()
		b.err = err
		b.mu.Unlock()
	}
}

type mappedBodyReader struct {
	*bytes.Reader
	release func()
	once    sync.Once
	mu      sync.Mutex
	closed  bool
}

func (r *mappedBodyReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.EOF
	}
	n, err := r.Reader.Read(p)
	if err != nil {
		r.closed = true
		r.once.Do(r.release)
	}
	return n, err
}

func (r *mappedBodyReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.once.Do(r.release)
	return nil
}
