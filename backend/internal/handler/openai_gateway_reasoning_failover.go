package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// openAIPassthroughFailoverState records whether the incoming conversation history
// may contain reasoning produced by a passthrough upstream. The flag is monotonic
// for the sticky session because clients can resend older turns in full.
type openAIPassthroughFailoverState struct {
	historyMayContainPassthrough bool
	historyKnown                 bool
}

// deriveOpenAIForwardAttemptBody returns the request body for the upcoming forward
// attempt against account. It always derives from the immutable canonical body and
// removes encrypted reasoning input item(s) when the session's full history may
// contain passthrough output and the upcoming account is non-passthrough. Missing
// provenance is treated conservatively during rollout: non-passthrough attempts
// sanitize any encrypted reasoning rather than risking an upstream rejection.
// The canonical slice is never mutated.
//
// This method is invoked exactly once per forward attempt, immediately before the
// Forward call.
func (h *OpenAIGatewayHandler) deriveOpenAIForwardAttemptBody(
	reqLog *zap.Logger,
	canonicalBody []byte,
	account *service.Account,
	state *openAIPassthroughFailoverState,
) []byte {
	return h.deriveOpenAIForwardAttemptBodyForMode(
		reqLog,
		canonicalBody,
		account,
		account.ShouldUseOpenAIResponsesPassthrough(),
		state,
	)
}

func (h *OpenAIGatewayHandler) deriveOpenAIForwardAttemptBodyForMode(
	reqLog *zap.Logger,
	canonicalBody []byte,
	account *service.Account,
	currentPassthrough bool,
	state *openAIPassthroughFailoverState,
) []byte {
	if currentPassthrough {
		return canonicalBody
	}
	if state != nil && state.historyKnown && !state.historyMayContainPassthrough {
		return canonicalBody
	}

	sanitized, changed, err := service.SanitizeOpenAICrossModeFailoverReasoning(canonicalBody)
	if err != nil {
		if reqLog != nil {
			reqLog.Warn("openai.failover_cross_mode_reasoning_sanitize_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
		}
		return canonicalBody
	}
	if !changed {
		return canonicalBody
	}
	if state != nil {
		state.historyMayContainPassthrough = true
		state.historyKnown = true
	}
	if reqLog != nil {
		reqLog.Info("openai.failover_cross_mode_reasoning_stripped",
			zap.Int64("account_id", account.ID),
			zap.Bool("account_passthrough", currentPassthrough),
			zap.Bool("reasoning_history_known", state != nil && state.historyKnown),
			zap.Bool("reasoning_history_may_contain_passthrough", state != nil && state.historyMayContainPassthrough),
		)
	}
	return sanitized
}
