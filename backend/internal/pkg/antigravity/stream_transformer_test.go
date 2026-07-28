//go:build unit

package antigravity

import (
	"encoding/json"
	"strings"
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

// Streaming conversion must replace non-empty Gemini IDs at the Anthropic boundary.
func TestStreamingProcessorUsesAnthropicMessageIDFormat(t *testing.T) {
	processor := NewStreamingProcessor("claude-sonnet-4-5")
	output, err := processor.ProcessLine(`data: {"responseId":"resp_wrapper","response":{"responseId":"resp_nested","candidates":[]}}`)
	require.NoError(t, err)

	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		require.Regexp(t, `^msg_01[0-9A-Za-z]{22}$`, event.Message.ID)
		return
	}
	t.Fatal("message_start data event not found")
}
