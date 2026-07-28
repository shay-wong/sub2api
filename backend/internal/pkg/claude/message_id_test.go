//go:build unit

package claude

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

var messageIDPattern = regexp.MustCompile(`^msg_01[0-9A-Za-z]{22}$`)

type failingEntropyReader struct{}

func (failingEntropyReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestGenerateMessageIDFormat(t *testing.T) {
	require.Regexp(t, messageIDPattern, GenerateMessageID())
}

// Entropy failure must not produce malformed or repeatable provider-facing IDs.
func TestGenerateMessageIDFallbackFormatAndUniqueness(t *testing.T) {
	first := generateMessageID(failingEntropyReader{})
	second := generateMessageID(failingEntropyReader{})

	require.Regexp(t, messageIDPattern, first)
	require.Regexp(t, messageIDPattern, second)
	require.NotEqual(t, first, second)
}
