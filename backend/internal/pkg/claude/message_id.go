package claude

import (
	"crypto/rand"
	"io"
	"sync/atomic"
	"time"
)

const messageIDCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var messageIDFallbackCounter atomic.Uint64

// GenerateMessageID returns an Anthropic-compatible message ID.
func GenerateMessageID() string {
	return generateMessageID(rand.Reader)
}

func generateMessageID(source io.Reader) string {
	const suffixLength = 22

	suffix := make([]byte, suffixLength)
	if _, err := io.ReadFull(source, suffix); err != nil {
		seed := uint64(time.Now().UnixNano()) ^ messageIDFallbackCounter.Add(1)
		for i := range suffix {
			seed ^= seed << 13
			seed ^= seed >> 7
			seed ^= seed << 17
			suffix[i] = byte(seed)
		}
	}
	for i := range suffix {
		suffix[i] = messageIDCharset[int(suffix[i])%len(messageIDCharset)]
	}
	return "msg_01" + string(suffix)
}
