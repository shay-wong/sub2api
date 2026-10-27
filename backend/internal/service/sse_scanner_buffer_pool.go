package service

import (
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

func disconnectedStreamDrainTimeout(streamTimeout time.Duration) time.Duration {
	if streamTimeout > 0 {
		return streamTimeout
	}
	return defaultDisconnectedStreamDrainTimeout
}
