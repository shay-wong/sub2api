package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type antigravityCompatStreamAdapter interface {
	Emit(*apicompat.AnthropicStreamEvent, *antigravityClientWriter) error
	Finalize(*antigravityClientWriter) error
	WriteError(*antigravityClientWriter, string)
	Completed() bool
}

type antigravityChatStreamAdapter struct {
	anthropicState *apicompat.AnthropicEventToResponsesState
	chatState      *apicompat.ResponsesEventToChatState
}

func newAntigravityChatStreamAdapter(model string, includeUsage bool) *antigravityChatStreamAdapter {
	anthropicState := apicompat.NewAnthropicEventToResponsesState()
	anthropicState.Model = model
	chatState := apicompat.NewResponsesEventToChatState()
	chatState.Model = model
	chatState.IncludeUsage = includeUsage
	return &antigravityChatStreamAdapter{
		anthropicState: anthropicState,
		chatState:      chatState,
	}
}

func (a *antigravityChatStreamAdapter) Emit(event *apicompat.AnthropicStreamEvent, writer *antigravityClientWriter) error {
	for _, responseEvent := range apicompat.AnthropicEventToResponsesEvents(event, a.anthropicState) {
		if err := a.emitResponseEvent(&responseEvent, writer); err != nil {
			return err
		}
	}
	return nil
}

func (a *antigravityChatStreamAdapter) Finalize(writer *antigravityClientWriter) error {
	for _, responseEvent := range apicompat.FinalizeAnthropicResponsesStream(a.anthropicState) {
		if err := a.emitResponseEvent(&responseEvent, writer); err != nil {
			return err
		}
	}
	for _, chunk := range apicompat.FinalizeResponsesChatStream(a.chatState) {
		data, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			return fmt.Errorf("encode final chat chunk: %w", err)
		}
		writer.Write([]byte(data))
	}
	writer.Write([]byte("data: [DONE]\n\n"))
	return nil
}

func (a *antigravityChatStreamAdapter) WriteError(writer *antigravityClientWriter, reason string) {
	writer.Fprintf("data: {\"error\":{\"message\":%q,\"type\":\"upstream_error\"}}\n\n", reason)
}

func (a *antigravityChatStreamAdapter) Completed() bool {
	return a.anthropicState.CompletedSent
}

func (a *antigravityChatStreamAdapter) emitResponseEvent(event *apicompat.ResponsesStreamEvent, writer *antigravityClientWriter) error {
	for _, chunk := range apicompat.ResponsesEventToChatChunks(event, a.chatState) {
		data, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			return fmt.Errorf("encode chat chunk: %w", err)
		}
		writer.Write([]byte(data))
	}
	return nil
}

type antigravityResponsesStreamAdapter struct {
	c              *gin.Context
	anthropicState *apicompat.AnthropicEventToResponsesState
	clientRestorer *apicompat.ResponsesClientToolStreamRestorer
}

func newAntigravityResponsesStreamAdapter(
	c *gin.Context,
	model string,
	mapping apicompat.ResponsesClientToolMapping,
) *antigravityResponsesStreamAdapter {
	state := apicompat.NewAnthropicEventToResponsesState()
	state.Model = model
	return &antigravityResponsesStreamAdapter{
		c:              c,
		anthropicState: state,
		clientRestorer: apicompat.NewResponsesClientToolStreamRestorer(mapping),
	}
}

func (a *antigravityResponsesStreamAdapter) Emit(event *apicompat.AnthropicStreamEvent, writer *antigravityClientWriter) error {
	for _, responseEvent := range apicompat.AnthropicEventToResponsesEvents(event, a.anthropicState) {
		if err := a.emitResponseEvent(responseEvent, writer); err != nil {
			return err
		}
	}
	return nil
}

func (a *antigravityResponsesStreamAdapter) Finalize(writer *antigravityClientWriter) error {
	for _, responseEvent := range apicompat.FinalizeAnthropicResponsesStream(a.anthropicState) {
		if err := a.emitResponseEvent(responseEvent, writer); err != nil {
			return err
		}
	}
	return nil
}

func (a *antigravityResponsesStreamAdapter) WriteError(writer *antigravityClientWriter, reason string) {
	message, _ := json.Marshal(reason)
	writer.Fprintf("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"object\":\"response\",\"status\":\"failed\",\"output\":[],\"error\":{\"code\":\"upstream_error\",\"message\":%s}}}\n\n", message)
}

func (a *antigravityResponsesStreamAdapter) Completed() bool {
	return a.anthropicState.CompletedSent
}

func (a *antigravityResponsesStreamAdapter) emitResponseEvent(event apicompat.ResponsesStreamEvent, writer *antigravityClientWriter) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode responses event: %w", err)
	}
	payload = reverseToolNamesIfPresent(a.c, payload)
	payloads, _, err := a.clientRestorer.RestoreEvent(payload)
	if err != nil {
		return fmt.Errorf("restore responses client tools: %w", err)
	}
	for _, restored := range payloads {
		eventType := gjson.GetBytes(restored, "type").String()
		writer.Fprintf("event: %s\ndata: %s\n\n", eventType, restored)
	}
	return nil
}

type antigravityCompatScanEvent struct {
	line string
	err  error
}

type antigravityCompatStreamSession struct {
	processor      *antigravity.StreamingProcessor
	adapter        antigravityCompatStreamAdapter
	writer         *antigravityClientWriter
	usage          *ClaudeUsage
	pendingEvents  []apicompat.AnthropicStreamEvent
	firstTokenMs   *int
	startTime      time.Time
	meaningfulData bool
}

func newAntigravityCompatStreamSession(
	model string,
	startTime time.Time,
	adapter antigravityCompatStreamAdapter,
	writer *antigravityClientWriter,
) *antigravityCompatStreamSession {
	return &antigravityCompatStreamSession{
		processor: antigravity.NewStreamingProcessor(model),
		adapter:   adapter,
		writer:    writer,
		usage:     &ClaudeUsage{},
		startTime: startTime,
	}
}

func (s *antigravityCompatStreamSession) consume(line string) error {
	claudeEvents, err := s.processor.ProcessLine(strings.TrimRight(line, "\r\n"))
	if err != nil {
		return err
	}
	if len(claudeEvents) == 0 {
		return nil
	}
	return s.consumeClaudeEvents(claudeEvents)
}

func (s *antigravityCompatStreamSession) hasMeaningfulData() bool {
	return s.meaningfulData
}

func (s *antigravityCompatStreamSession) finish() (*antigravityStreamResult, error) {
	finalEvents, usage := s.processor.Finish()
	mergeAntigravityCompatUsage(s.usage, usage)
	if err := s.consumeClaudeEvents(finalEvents); err != nil {
		return s.result(s.writer.Disconnected()), err
	}
	if err := s.adapter.Finalize(s.writer); err != nil {
		return s.result(s.writer.Disconnected()), err
	}
	return s.result(s.writer.Disconnected()), nil
}

func (s *antigravityCompatStreamSession) collectResult(clientDisconnect bool) *antigravityStreamResult {
	_, usage := s.processor.Finish()
	mergeAntigravityCompatUsage(s.usage, usage)
	return s.result(clientDisconnect)
}

func (s *antigravityCompatStreamSession) result(clientDisconnect bool) *antigravityStreamResult {
	return &antigravityStreamResult{
		usage:            s.usage,
		firstTokenMs:     s.firstTokenMs,
		clientDisconnect: clientDisconnect,
	}
}

func (s *antigravityCompatStreamSession) consumeClaudeEvents(data []byte) error {
	var eventType string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if err := s.consumeClaudeData(eventType, strings.TrimSpace(strings.TrimPrefix(line, "data:"))); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *antigravityCompatStreamSession) consumeClaudeData(eventType, payload string) error {
	var event apicompat.AnthropicStreamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("decode generated anthropic event: %w", err)
	}
	if event.Type == "" {
		event.Type = eventType
	}
	if event.Usage != nil {
		mergeAnthropicUsage(s.usage, *event.Usage)
	}
	if event.Message != nil {
		mergeAnthropicUsage(s.usage, event.Message.Usage)
	}
	return s.emitOrBuffer(event)
}

func (s *antigravityCompatStreamSession) emitOrBuffer(event apicompat.AnthropicStreamEvent) error {
	if s.meaningfulData {
		return s.adapter.Emit(&event, s.writer)
	}

	s.pendingEvents = append(s.pendingEvents, event)
	if !isMeaningfulAntigravityCompatEvent(&event) {
		return nil
	}

	s.meaningfulData = true
	ms := int(time.Since(s.startTime).Milliseconds())
	s.firstTokenMs = &ms
	for i := range s.pendingEvents {
		if err := s.adapter.Emit(&s.pendingEvents[i], s.writer); err != nil {
			return err
		}
	}
	s.pendingEvents = nil
	return nil
}

func isMeaningfulAntigravityCompatEvent(event *apicompat.AnthropicStreamEvent) bool {
	if event == nil {
		return false
	}
	if event.Type == "message_stop" {
		return true
	}
	if event.ContentBlock != nil {
		block := event.ContentBlock
		return block.Type == "tool_use" ||
			block.Text != "" ||
			block.Thinking != "" ||
			block.Signature != "" ||
			block.Source != nil
	}
	if event.Delta != nil {
		delta := event.Delta
		return delta.Text != "" ||
			delta.PartialJSON != "" ||
			delta.Thinking != "" ||
			delta.Signature != "" ||
			delta.StopReason != ""
	}
	return false
}

func mergeAntigravityCompatUsage(dst *ClaudeUsage, src *antigravity.ClaudeUsage) {
	if dst == nil || src == nil {
		return
	}
	dst.InputTokens = src.InputTokens
	dst.OutputTokens = src.OutputTokens
	dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	dst.CacheReadInputTokens = src.CacheReadInputTokens
	dst.ImageOutputTokens = src.ImageOutputTokens
}

func (s *AntigravityGatewayService) handleAntigravityCompatStream(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	adapter antigravityCompatStreamAdapter,
	prefix string,
) (*antigravityStreamResult, error) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	writer := newAntigravityClientWriter(c.Writer, flusher, prefix)
	writer.beforeFirstWrite = func() {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
	}
	session := newAntigravityCompatStreamSession(originalModel, startTime, adapter, writer)
	events, stopScanner, maxLineSize := s.startAntigravityCompatScanner(resp.Body)
	defer stopScanner()

	timeout := s.antigravityCompatStreamTimeout()
	timeoutTimer, timeoutCh := newStreamDataTimer(timeout)
	if timeoutTimer != nil {
		defer timeoutTimer.Stop()
	}
	keepaliveTicker, keepaliveCh := s.newAntigravityCompatKeepaliveTicker()
	if keepaliveTicker != nil {
		defer keepaliveTicker.Stop()
	}
	var clientDone <-chan struct{}
	if c.Request != nil {
		clientDone = c.Request.Context().Done()
	}
	var disconnectDrainTimer *time.Timer
	var disconnectDrainCh <-chan time.Time
	startDisconnectDrain := func() {
		if disconnectDrainTimer != nil {
			return
		}
		disconnectDrainTimer = time.NewTimer(disconnectedStreamDrainTimeout(timeout))
		disconnectDrainCh = disconnectDrainTimer.C
	}
	defer func() {
		if disconnectDrainTimer != nil {
			disconnectDrainTimer.Stop()
		}
	}()

	for {
		select {
		case event, open := <-events:
			if !open {
				if !session.hasMeaningfulData() && !writer.Disconnected() {
					return nil, antigravityCompatEmptyStreamError()
				}
				result, err := session.finish()
				if err != nil {
					return s.handleAntigravityCompatConversionError(c, session, result, err, prefix)
				}
				return result, nil
			}
			if event.err != nil {
				return s.handleAntigravityCompatReadError(c, session, event.err, maxLineSize, prefix)
			}
			resetStreamDataTimer(timeoutTimer, timeout)
			s.observeAntigravityGeminiSSELine(c, event.line)
			wasDisconnected := writer.Disconnected()
			if err := session.consume(event.line); err != nil {
				return s.handleAntigravityCompatConversionError(c, session, nil, err, prefix)
			}
			if adapter.Completed() {
				result, err := session.finish()
				if err != nil {
					return s.handleAntigravityCompatConversionError(c, session, result, err, prefix)
				}
				return result, nil
			}
			if !wasDisconnected && writer.Disconnected() {
				startDisconnectDrain()
			}

		case <-timeoutCh:
			if writer.Disconnected() {
				return session.collectResult(true), nil
			}
			if !session.hasMeaningfulData() {
				return nil, antigravityCompatEmptyStreamError()
			}
			logger.LegacyPrintf("service.antigravity_gateway", "Stream data interval timeout (%s)", prefix)
			writeAntigravityCompatStreamError(c, adapter, writer, "stream_timeout")
			return session.collectResult(false), fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if session.hasMeaningfulData() && !writer.Disconnected() {
				writer.Write([]byte(": ping\n\n"))
			}

		case <-clientDone:
			clientDone = nil
			if !writer.Disconnected() {
				writer.markDisconnected()
			}
			startDisconnectDrain()

		case <-disconnectDrainCh:
			return session.collectResult(true), nil
		}
	}
}

func (s *AntigravityGatewayService) handleAntigravityCompatConversionError(
	c *gin.Context,
	session *antigravityCompatStreamSession,
	result *antigravityStreamResult,
	err error,
	prefix string,
) (*antigravityStreamResult, error) {
	logger.L().Warn("antigravity compatibility stream conversion failed",
		zap.String("request_id", c.Writer.Header().Get("x-request-id")),
		zap.String("stream", prefix),
		zap.Error(err),
	)
	if session.writer.Disconnected() {
		if result == nil {
			result = session.collectResult(true)
		}
		return result, nil
	}
	if !session.writer.Started() {
		return nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			ResponseBody:           []byte(`{"error":"failed to convert upstream stream"}`),
			RetryableOnSameAccount: true,
		}
	}
	writeAntigravityCompatStreamError(c, session.adapter, session.writer, "stream_conversion_error")
	if result == nil {
		result = session.collectResult(false)
	}
	return result, fmt.Errorf("convert upstream stream: %w", err)
}

func (s *AntigravityGatewayService) startAntigravityCompatScanner(
	body io.Reader,
) (<-chan antigravityCompatScanEvent, func(), int) {
	maxLineSize := defaultMaxLineSize
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.settingService.cfg.Gateway.MaxLineSize
	}
	scanner := bufio.NewScanner(body)
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	events := make(chan antigravityCompatScanEvent, 16)
	done := make(chan struct{})
	go func() {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		send := func(event antigravityCompatScanEvent) bool {
			select {
			case events <- event:
				return true
			case <-done:
				return false
			}
		}
		for scanner.Scan() {
			if !send(antigravityCompatScanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			send(antigravityCompatScanEvent{err: err})
		}
	}()
	return events, func() { close(done) }, maxLineSize
}

func (s *AntigravityGatewayService) antigravityCompatStreamTimeout() time.Duration {
	if s.settingService == nil || s.settingService.cfg == nil {
		return 0
	}
	return time.Duration(s.settingService.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
}

func (s *AntigravityGatewayService) newAntigravityCompatKeepaliveTicker() (*time.Ticker, <-chan time.Time) {
	if s.settingService == nil || s.settingService.cfg == nil {
		return nil, nil
	}
	interval := time.Duration(s.settingService.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	if interval <= 0 {
		return nil, nil
	}
	ticker := time.NewTicker(interval)
	return ticker, ticker.C
}

func newStreamDataTimer(timeout time.Duration) (*time.Timer, <-chan time.Time) {
	if timeout <= 0 {
		return nil, nil
	}
	timer := time.NewTimer(timeout)
	return timer, timer.C
}

func resetStreamDataTimer(timer *time.Timer, timeout time.Duration) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(timeout)
}

func (s *AntigravityGatewayService) handleAntigravityCompatReadError(
	c *gin.Context,
	session *antigravityCompatStreamSession,
	err error,
	maxLineSize int,
	prefix string,
) (*antigravityStreamResult, error) {
	if !session.hasMeaningfulData() && !session.writer.Disconnected() {
		return nil, antigravityCompatEmptyStreamError()
	}
	if disconnect, handled := handleStreamReadError(err, session.writer.Disconnected(), prefix); handled {
		return session.collectResult(disconnect), nil
	}
	if errors.Is(err, bufio.ErrTooLong) {
		logger.LegacyPrintf("service.antigravity_gateway", "SSE line too long (%s): max_size=%d error=%v", prefix, maxLineSize, err)
		writeAntigravityCompatStreamError(c, session.adapter, session.writer, "response_too_large")
		return session.result(false), err
	}
	writeAntigravityCompatStreamError(c, session.adapter, session.writer, "stream_read_error")
	return nil, fmt.Errorf("stream read error: %w", err)
}

func writeAntigravityCompatStreamError(
	c *gin.Context,
	adapter antigravityCompatStreamAdapter,
	writer *antigravityClientWriter,
	reason string,
) {
	adapter.WriteError(writer, reason)
	MarkResponseCommitted(c)
}

func antigravityCompatEmptyStreamError() error {
	logger.LegacyPrintf("service.antigravity_gateway", "Empty Antigravity compatibility stream, triggering failover")
	return &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           []byte(`{"error":"empty stream response from upstream"}`),
		RetryableOnSameAccount: true,
	}
}

func (s *AntigravityGatewayService) handleChatCompletionsStreamingFromAntigravity(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	includeUsage bool,
) (*antigravityStreamResult, error) {
	return s.handleAntigravityCompatStream(
		c,
		resp,
		startTime,
		originalModel,
		newAntigravityChatStreamAdapter(originalModel, includeUsage),
		"antigravity chat completions stream",
	)
}

func (s *AntigravityGatewayService) handleResponsesStreamingFromAntigravity(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	clientToolMapping apicompat.ResponsesClientToolMapping,
) (*antigravityStreamResult, error) {
	return s.handleAntigravityCompatStream(
		c,
		resp,
		startTime,
		originalModel,
		newAntigravityResponsesStreamAdapter(c, originalModel, clientToolMapping),
		"antigravity responses stream",
	)
}
