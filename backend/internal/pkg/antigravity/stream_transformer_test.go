//go:build unit

package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamingProcessorProcessLineRejectsInvalidAndErrorPayloads(t *testing.T) {
	for _, line := range []string{
		"data: not-json",
		`data: {"error":{"message":"upstream overloaded"}}`,
	} {
		processor := NewStreamingProcessor("gemini-test")
		output, err := processor.ProcessLine(line)

		require.Error(t, err)
		require.Empty(t, output)
	}
}
