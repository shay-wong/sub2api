package service

import (
	"bufio"
	"io"
	"sync"
	"time"
)

const sseScannerBuf64KSize = 64 * 1024
const defaultDisconnectedStreamDrainTimeout = 30 * time.Second

type sseScannerBuf64K [sseScannerBuf64KSize]byte

var sseScannerBuf64KPool = sync.Pool{
	New: func() any {
		return new(sseScannerBuf64K)
	},
}

func getSSEScannerBuf64K() *sseScannerBuf64K {
	v := sseScannerBuf64KPool.Get()
	buf, ok := v.(*sseScannerBuf64K)
	if !ok || buf == nil {
		return new(sseScannerBuf64K)
	}
	return buf
}

func putSSEScannerBuf64K(buf *sseScannerBuf64K) {
	if buf == nil {
		return
	}
	sseScannerBuf64KPool.Put(buf)
}

type sseLineScanEvent struct {
	line string
	err  error
}

func startSSELineScanner(r io.Reader, maxLineSize int) (<-chan sseLineScanEvent, func()) {
	scanner := bufio.NewScanner(r)
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	events := make(chan sseLineScanEvent, 16)
	done := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		send := func(event sseLineScanEvent) bool {
			select {
			case events <- event:
				return true
			case <-done:
				return false
			}
		}
		for scanner.Scan() {
			if !send(sseLineScanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			send(sseLineScanEvent{err: err})
		}
	}()

	return events, func() { stopOnce.Do(func() { close(done) }) }
}

func disconnectedStreamDrainTimeout(streamTimeout time.Duration) time.Duration {
	if streamTimeout > 0 {
		return streamTimeout
	}
	return defaultDisconnectedStreamDrainTimeout
}
