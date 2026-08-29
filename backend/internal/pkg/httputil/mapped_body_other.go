//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package httputil

import (
	"bytes"
	"io"
)

// NewMappedBody stores bytes in memory on platforms without mmap support.
func NewMappedBody(write func(io.Writer) error) (*MappedBody, error) {
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		return nil, err
	}
	return newMappedBody(buf.Bytes(), nil), nil
}
