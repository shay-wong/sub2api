//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package httputil

import (
	"io"
	"os"
	"syscall"
)

// NewMappedBody writes bytes to a temporary file and maps them read-only.
func NewMappedBody(write func(io.Writer) error) (*MappedBody, error) {
	file, err := os.CreateTemp("", "sub2api-body-*")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	cleanupFile := func() {
		_ = file.Close()
		_ = os.Remove(path)
	}

	if err := write(file); err != nil {
		cleanupFile()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		cleanupFile()
		return nil, err
	}
	if info.Size() == 0 {
		cleanupFile()
		return newMappedBody([]byte{}, nil), nil
	}
	if info.Size() > int64(int(^uint(0)>>1)) {
		cleanupFile()
		return nil, syscall.EFBIG
	}

	data, err := syscall.Mmap(int(file.Fd()), 0, int(info.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		cleanupFile()
		return nil, err
	}
	if err := os.Remove(path); err != nil {
		_ = syscall.Munmap(data)
		cleanupFile()
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = syscall.Munmap(data)
		return nil, err
	}
	return newMappedBody(data, func() error { return syscall.Munmap(data) }), nil
}
