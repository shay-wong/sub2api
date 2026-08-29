//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleCCBufferedFromAnthropic_ToolArgumentsAreValidJSON(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_tool","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":10}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
		``,
	}, "\n")))}

	_, err := (&GatewayService{}).handleCCBufferedFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4.5", nil, time.Now())
	require.NoError(t, err)

	var body struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Choices, 1)
	require.Len(t, body.Choices[0].Message.ToolCalls, 1)
	args := body.Choices[0].Message.ToolCalls[0].Function.Arguments
	require.JSONEq(t, `{"city":"Paris"}`, args)
}

func TestExtractCCReasoningEffortFromBody(t *testing.T) {
	t.Parallel()

	t.Run("nested reasoning.effort", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning":{"effort":"HIGH"}}`))
		require.NotNil(t, got)
		require.Equal(t, "high", *got)
	})

	t.Run("flat reasoning_effort", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"reasoning_effort":"x-high"}`))
		require.NotNil(t, got)
		require.Equal(t, "xhigh", *got)
	})

	t.Run("DeepSeek max", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"model":"deepseek-v4-flash","reasoning_effort":"Max"}`))
		require.NotNil(t, got)
		require.Equal(t, "max", *got)
	})

	t.Run("mapped Kimi alias max", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody(
			[]byte(`{"model":"public-alias","reasoning_effort":"max"}`),
			"kimi-k3",
			"public-alias",
		)
		require.NotNil(t, got)
		require.Equal(t, "max", *got)
	})

	t.Run("legacy model max", func(t *testing.T) {
		got := extractCCReasoningEffortFromBody([]byte(`{"model":"gpt-5.5","reasoning_effort":"max"}`))
		require.NotNil(t, got)
		require.Equal(t, "xhigh", *got)
	})

	t.Run("missing effort", func(t *testing.T) {
		require.Nil(t, extractCCReasoningEffortFromBody([]byte(`{"model":"gpt-5"}`)))
	})
}

func TestHandleCCBufferedFromAnthropic_PreservesMessageStartCacheUsageAndReasoning(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	reasoningEffort := "high"
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_buffered"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":12,"cache_read_input_tokens":9,"cache_creation_input_tokens":3}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCBufferedFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4.5", &reasoningEffort, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 9, result.Usage.CacheReadInputTokens)
	require.Equal(t, 3, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
}

// Kimi 等 Anthropic 兼容上游返回 SSE 紧凑格式（冒号后无空格），CC 桥此前按
// "event: " / "data: " 严格匹配会丢弃全部事件，最终报 "Upstream stream ended
// without a response"（#4653 同根因；#4657 只修了 /v1/responses 桥）。
func TestHandleCCBufferedFromAnthropic_CompactSSEFormat(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_buffered_compact"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event:message_start`,
			`data:{"type":"message_start","message":{"id":"msg_c1","type":"message","role":"assistant","content":[],"model":"k3","stop_reason":"","usage":{"input_tokens":15,"cache_read_input_tokens":5,"cache_creation_input_tokens":2}}}`,
			``,
			`event:content_block_start`,
			`data:{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"OK"}}`,
			``,
			`event:message_delta`,
			`data:{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCBufferedFromAnthropic(resp, c, "k3", "k3", nil, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 15, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)
	require.Equal(t, 2, result.Usage.CacheCreationInputTokens)
	require.Contains(t, rec.Body.String(), `"OK"`, "紧凑格式事件必须被解析并产出响应内容")
}

func TestHandleCCStreamingFromAnthropic_CompactSSEFormat(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream_compact"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event:message_start`,
			`data:{"type":"message_start","message":{"id":"msg_c2","type":"message","role":"assistant","content":[],"model":"k3","stop_reason":"","usage":{"input_tokens":21,"cache_read_input_tokens":6,"cache_creation_input_tokens":1}}}`,
			``,
			`event:content_block_start`,
			`data:{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"OK"}}`,
			``,
			`event:message_delta`,
			`data:{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
			``,
			`event:message_stop`,
			`data:{"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "k3", "k3", nil, time.Now(), true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 21, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 6, result.Usage.CacheReadInputTokens)
	require.Equal(t, 1, result.Usage.CacheCreationInputTokens)
	require.Contains(t, rec.Body.String(), `[DONE]`)
}

func TestHandleCCStreamingFromAnthropic_PreservesMessageStartCacheUsageAndReasoning(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	reasoningEffort := "medium"
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_cc_stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":"","usage":{"input_tokens":20,"cache_read_input_tokens":11,"cache_creation_input_tokens":4}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hello"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}

	svc := &GatewayService{}
	result, err := svc.handleCCStreamingFromAnthropic(resp, c, "gpt-5", "claude-sonnet-4.5", &reasoningEffort, time.Now(), true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 20, result.Usage.InputTokens)
	require.Equal(t, 8, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, 4, result.Usage.CacheCreationInputTokens)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "medium", *result.ReasoningEffort)
	require.Contains(t, rec.Body.String(), `[DONE]`)
}

// A broken upstream stream must not turn an already-created partial message into
// a successful buffered Chat Completions response.
func TestHandleCCBufferedFromAnthropic_RejectsBrokenUpstreamStream(t *testing.T) {
	messageStart := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_partial\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-fable-5\",\"usage\":{\"input_tokens\":1}}}\n\n"
	tests := []struct {
		name string
		body func() io.ReadCloser
	}{
		{name: "malformed json", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: message_delta\ndata: not-json\n\n"))
		}},
		{name: "anthropic error", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"upstream overloaded\"}}\n\n"))
		}},
		{name: "missing data", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: message_delta\nid: orphaned\n\n"))
		}},
		{name: "empty data", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: message_delta\ndata: \t \n\n"))
		}},
		{name: "read failure", body: func() io.ReadCloser {
			return &errTailReader{data: []byte(messageStart), err: io.ErrUnexpectedEOF}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			result, err := (&GatewayService{}).handleCCBufferedFromAnthropic(
				&http.Response{Body: tt.body()}, c, "gpt-5", "claude-fable-5", nil, time.Now(),
			)

			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.Contains(t, rec.Body.String(), `"type":"stream_conversion_error"`)
		})
	}
}

// Once a Chat Completions SSE response is committed, protocol failures must end
// with an error frame and never the normal [DONE] marker.
func TestHandleCCStreamingFromAnthropic_RejectsBrokenUpstreamStream(t *testing.T) {
	messageStart := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_partial\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-fable-5\",\"usage\":{\"input_tokens\":1}}}\n\n"
	tests := []struct {
		name string
		body func() io.ReadCloser
	}{
		{name: "malformed json", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: message_delta\ndata: not-json\n\n"))
		}},
		{name: "anthropic error", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"upstream overloaded\"}}\n\n"))
		}},
		{name: "missing data", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: message_delta\nid: orphaned\n\n"))
		}},
		{name: "empty data", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(messageStart + "event: message_delta\ndata: \t \n\n"))
		}},
		{name: "read failure", body: func() io.ReadCloser {
			return &errTailReader{data: []byte(messageStart), err: io.ErrUnexpectedEOF}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
				&http.Response{Body: tt.body()}, c, "gpt-5", "claude-fable-5", nil, time.Now(), false,
			)

			require.Error(t, err)
			require.NotNil(t, result)
			require.True(t, IsResponseCommitted(c))
			require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"stream_conversion_error"`))
			require.Contains(t, rec.Body.String(), `"type":"stream_conversion_error"`)
			require.NotContains(t, rec.Body.String(), `[DONE]`)
		})
	}
}

func TestHandleCCStreamingFromAnthropic_RequestCancelDrainsLateUsage(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)
	reader, writer := io.Pipe()
	defer reader.Close()
	type outcome struct {
		result *ForwardResult
		err    error
	}
	done := make(chan outcome, 1)

	go func() {
		result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
			&http.Response{Body: reader}, c, "gpt-5", "claude-sonnet-4.5", nil, time.Now(), true,
		)
		done <- outcome{result: result, err: err}
	}()
	cancel()
	time.Sleep(20 * time.Millisecond)
	_, err := io.WriteString(writer, strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_cancel","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":12}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
		``,
	}, "\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	got := <-done
	require.NoError(t, got.err)
	require.NotNil(t, got.result)
	require.True(t, got.result.ClientDisconnect)
	require.Equal(t, 12, got.result.Usage.InputTokens)
	require.Equal(t, 7, got.result.Usage.OutputTokens)
}

func TestHandleCCStreamingFromAnthropic_RequestCancelHasConfiguredDrainDeadline(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}}
	done := make(chan *ForwardResult, 1)

	go func() {
		result, _ := svc.handleCCStreamingFromAnthropic(
			&http.Response{Body: reader}, c, "gpt-5", "claude-sonnet-4.5", nil, time.Now(), true,
		)
		done <- result
	}()
	cancel()

	select {
	case result := <-done:
		require.NotNil(t, result)
		require.True(t, result.ClientDisconnect)
	case <-time.After(2 * time.Second):
		t.Fatal("request cancellation kept the Anthropic chat stream open past its drain deadline")
	}
}

func TestHandleCCStreamingFromAnthropic_ClientDisconnectDrainsLateUsage(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAIChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_disconnect","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":12,"cache_read_input_tokens":4}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")))}

	result, err := (&GatewayService{}).handleCCStreamingFromAnthropic(
		resp, c, "gpt-5", "claude-sonnet-4.5", nil, time.Now(), true,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
}

func TestHandleCCStreamingFromAnthropic_ClientDisconnectHasDrainDeadline(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAIChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}}
	done := make(chan *ForwardResult, 1)

	go func() {
		result, _ := svc.handleCCStreamingFromAnthropic(
			&http.Response{Body: reader}, c, "gpt-5", "claude-sonnet-4.5", nil, time.Now(), true,
		)
		done <- result
	}()
	_, err := io.WriteString(writer, strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_disconnect","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`,
		``,
	}, "\n")+"\n")
	require.NoError(t, err)

	select {
	case result := <-done:
		require.NotNil(t, result)
		require.True(t, result.ClientDisconnect)
	case <-time.After(2 * time.Second):
		t.Fatal("client disconnect kept the Anthropic chat stream open past its drain deadline")
	}
}
