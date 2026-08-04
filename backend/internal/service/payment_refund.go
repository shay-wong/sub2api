package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/google/uuid"
)

// --- Refund Flow ---

var createPaymentProviderFromInstance = provider.CreateProvider

const (
	refundProviderCallTimeout   = 4 * time.Minute
	refundRecoveryQueryTimeout  = 1 * time.Minute
	refundRecoveryReplayTimeout = 3 * time.Minute
	refundRecoveryDelay         = 5 * time.Minute
)

var errRefundForceRequired = errors.New("refund balance deduction requires force")

// getOrderProviderInstance looks up the provider instance that processed this order.
// For legacy orders without provider_instance_id, it resolves only when the
// historical instance is uniquely identifiable from the stored order fields.
func (s *PaymentService) getOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return s.resolveUniqueLegacyOrderProviderInstance(ctx, o)
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	return s.entClient.PaymentProviderInstance.Get(ctx, instID)
}

// getRefundOrderProviderInstance resolves the provider instance for refund paths.
// Refunds must be pinned to an explicit historical binding, so legacy
// "best-effort" provider guessing is intentionally not allowed here.
func (s *PaymentService) getRefundOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return nil, nil
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("order %d refund provider instance id is invalid: %s", o.ID, instIDStr)
	}
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d refund provider instance %s is missing", o.ID, instIDStr)
		}
		return nil, err
	}
	return inst, nil
}

func (s *PaymentService) resolveUniqueLegacyOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	paymentType := payment.GetBasePaymentType(strings.TrimSpace(o.PaymentType))
	providerKey := strings.TrimSpace(psStringValue(o.ProviderKey))
	if providerKey != "" {
		instances, err := s.entClient.PaymentProviderInstance.Query().
			Where(paymentproviderinstance.ProviderKeyEQ(providerKey)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
		if len(matched) == 1 {
			return matched[0], nil
		}
		return nil, nil
	}

	if paymentType == "" {
		return nil, nil
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		All(ctx)
	if err != nil {
		return nil, err
	}

	matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
	if len(matched) == 1 {
		return matched[0], nil
	}
	return nil, nil
}

func psFilterLegacyOrderProviderInstances(orderPaymentType string, instances []*dbent.PaymentProviderInstance) []*dbent.PaymentProviderInstance {
	if len(instances) == 0 {
		return nil
	}
	if strings.TrimSpace(orderPaymentType) == "" {
		return instances
	}
	var matched []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if psLegacyOrderMatchesInstance(orderPaymentType, inst) {
			matched = append(matched, inst)
		}
	}
	return matched
}

func psLegacyOrderMatchesInstance(orderPaymentType string, inst *dbent.PaymentProviderInstance) bool {
	if inst == nil {
		return false
	}

	baseType := payment.GetBasePaymentType(strings.TrimSpace(orderPaymentType))
	instanceProviderKey := strings.TrimSpace(inst.ProviderKey)
	if baseType == "" {
		return false
	}

	if baseType == payment.TypeStripe {
		return instanceProviderKey == payment.TypeStripe
	}
	if instanceProviderKey == payment.TypeStripe {
		return false
	}
	if instanceProviderKey == baseType {
		return true
	}
	return payment.InstanceSupportsType(inst.SupportedTypes, baseType)
}

func (s *PaymentService) RequestRefund(ctx context.Context, oid, uid int64, reason string) error {
	o, err := s.validateRefundRequest(ctx, oid, uid)
	if err != nil {
		return err
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u.Balance < o.Amount {
		return infraerrors.BadRequest("BALANCE_NOT_ENOUGH", "refund amount exceeds balance")
	}
	nr := strings.TrimSpace(reason)
	now := time.Now()
	by := fmt.Sprintf("%d", uid)
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.UserIDEQ(uid), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.OrderTypeEQ(payment.OrderTypeBalance)).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(now).SetRefundRequestReason(nr).SetRefundRequestedBy(by).SetRefundAmount(o.Amount).Save(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if c == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	s.writeAuditLog(ctx, oid, "REFUND_REQUESTED", fmt.Sprintf("user:%d", uid), map[string]any{"amount": o.Amount, "reason": nr})
	return nil
}

func (s *PaymentService) validateRefundRequest(ctx context.Context, oid, uid int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != uid {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission")
	}
	if o.OrderType != payment.OrderTypeBalance {
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "only balance orders can request refund")
	}
	if o.Status != OrderStatusCompleted {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only completed orders can request refund")
	}
	// Check provider instance allows user refund
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil || inst == nil {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.AllowUserRefund {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "user refund is not enabled for this provider")
	}
	return o, nil
}

func (s *PaymentService) PrepareRefund(ctx context.Context, oid int64, amt float64, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	ok := []string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed}
	if !psSliceContains(ok, o.Status) {
		return nil, nil, infraerrors.BadRequest("INVALID_STATUS", "order status does not allow refund")
	}
	previousAttemptID := ""
	if o.Status == OrderStatusRefundFailed {
		if err := s.ensureRefundRetryAllowed(ctx, o.ID); err != nil {
			return nil, nil, err
		}
		pendingDetail, err := s.latestRefundPendingDetail(ctx, o.ID)
		if err != nil {
			return nil, nil, err
		}
		if !pendingDetail.HasPendingAudit {
			return nil, nil, refundReconciliationRequired(errors.New("refund pending audit is unavailable"))
		}
		amt = o.RefundAmount
		reason = strings.TrimSpace(psStringValue(o.RefundReason))
		force = o.ForceRefund
		deduct = pendingDetail.DeductBalance
		previousAttemptID = pendingDetail.AttemptID
	}
	// Check provider instance allows admin refund
	inst, instErr := s.getRefundOrderProviderInstance(ctx, o)
	if instErr != nil {
		slog.Warn("refund: provider instance lookup failed", "orderID", oid, "error", instErr)
		return nil, nil, infraerrors.InternalServer("PROVIDER_LOOKUP_FAILED", "failed to look up payment provider for this order")
	}
	if inst == nil {
		// Legacy order without provider_instance_id — block refund
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.RefundEnabled {
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not enabled for this provider")
	}
	if math.IsNaN(amt) || math.IsInf(amt, 0) {
		return nil, nil, infraerrors.BadRequest("INVALID_AMOUNT", "invalid refund amount")
	}
	if amt <= 0 {
		amt = o.Amount
	}
	orderCurrency := PaymentOrderCurrency(o)
	if amt-o.Amount > paymentAmountToleranceForCurrency(orderCurrency) {
		return nil, nil, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds recharge")
	}
	ga := calculateGatewayRefundAmount(o.Amount, o.PayAmount, amt, orderCurrency)
	rr := strings.TrimSpace(reason)
	if rr == "" && o.RefundRequestReason != nil {
		rr = *o.RefundRequestReason
	}
	if rr == "" {
		rr = fmt.Sprintf("refund order:%d", o.ID)
	}
	p := &RefundPlan{
		OrderID:           oid,
		Order:             o,
		PreviousStatus:    o.Status,
		PreviousAttemptID: previousAttemptID,
		RefundAmount:      amt,
		GatewayAmount:     ga,
		Reason:            rr,
		Force:             force,
		DeductBalance:     deduct,
		DeductionType:     payment.DeductionTypeNone,
	}
	if deduct {
		if er := s.prepDeduct(ctx, o, p, force); er != nil {
			return nil, er, nil
		}
	}
	return p, nil, nil
}

func (s *PaymentService) prepDeduct(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if o.OrderType == payment.OrderTypeSubscription {
		p.DeductionType = payment.DeductionTypeSubscription
		if o.SubscriptionGroupID != nil && o.SubscriptionDays != nil {
			p.SubDaysToDeduct = *o.SubscriptionDays
			sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID)
			if err == nil && sub != nil {
				p.SubscriptionID = sub.ID
			} else if !force {
				return &RefundResult{Success: false, Warning: "cannot find active subscription for deduction, use force", RequireForce: true}
			}
		}
		return nil
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		if !force {
			return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
		}
		return nil
	}
	p.DeductionType = payment.DeductionTypeBalance
	if u.Balance < p.RefundAmount && !force {
		return &RefundResult{Success: false, Warning: "user balance is insufficient for deduction, use force", RequireForce: true}
	}
	p.BalanceToDeduct = math.Max(0, math.Min(p.RefundAmount, u.Balance))
	return nil
}

type availableBalanceDeductor interface {
	DeductAvailableBalance(ctx context.Context, id int64, amount float64) (float64, error)
}

func (s *PaymentService) deductAvailableBalance(ctx context.Context, userID int64, amount float64) (float64, error) {
	repo, ok := s.userRepo.(availableBalanceDeductor)
	if !ok {
		return 0, errors.New("user repository does not support available balance deduction")
	}
	return repo.DeductAvailableBalance(ctx, userID, amount)
}

func (s *PaymentService) ExecuteRefund(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	if err := s.ensureRefundRetryAllowed(ctx, p.OrderID); err != nil {
		return nil, err
	}
	prov, err := s.prepareRefundProvider(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := s.beginRefundAttempt(ctx, p); err != nil {
		if errors.Is(err, errRefundForceRequired) {
			return &RefundResult{Success: false, Warning: "user balance is insufficient for deduction, use force", RequireForce: true}, nil
		}
		return nil, err
	}
	resp, err := s.dispatchRefund(ctx, p, prov)
	return s.settleRefundProviderOutcome(ctx, p, resp, err)
}

func (s *PaymentService) beginRefundAttempt(ctx context.Context, p *RefundPlan) (err error) {
	if p.PreviousStatus == "" && p.Order != nil {
		p.PreviousStatus = p.Order.Status
	}
	if !psSliceContains([]string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed}, p.PreviousStatus) {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin refund attempt: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	claimed, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(p.PreviousStatus)).
		SetStatus(OrderStatusRefunding).
		Save(txCtx)
	if err != nil {
		return fmt.Errorf("claim refund attempt: %w", err)
	}
	if claimed == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if p.PreviousStatus == OrderStatusRefundFailed {
		latestDetail, err := latestRefundPendingDetailWithClient(txCtx, tx.Client(), p.OrderID)
		if err != nil {
			return err
		}
		if !latestDetail.HasPendingAudit {
			return refundReconciliationRequired(errors.New("refund pending audit is unavailable"))
		}
		if latestDetail.AttemptID != strings.TrimSpace(p.PreviousAttemptID) {
			return infraerrors.Conflict("CONFLICT", "refund attempt changed")
		}
	}

	p.AttemptID = uuid.NewString()
	p.ProviderRequestID = uuid.NewString()
	p.AttemptStartedAt = time.Now().UTC()
	if _, err := tx.PaymentOrder.UpdateOneID(p.OrderID).
		SetRefundAmount(p.RefundAmount).
		SetRefundReason(p.Reason).
		SetForceRefund(p.Force).
		ClearFailedAt().
		ClearFailedReason().
		Save(txCtx); err != nil {
		return fmt.Errorf("persist refund attempt parameters: %w", err)
	}

	mutatedSubscription, err := s.applyRefundDeduction(txCtx, p)
	if err != nil {
		return err
	}
	p.DeductionApplied = true
	if err = s.upsertRefundAudit(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", refundAttemptAuditDetail(p, "", false, "dispatching")); err != nil {
		return fmt.Errorf("persist refund attempt: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit refund attempt: %w", err)
	}
	tx = nil
	s.invalidateRefundSubscriptionCaches(p.OrderID, mutatedSubscription, "attempt deduction")
	return nil
}

func (s *PaymentService) prepareRefundProvider(ctx context.Context, p *RefundPlan) (payment.Provider, error) {
	if p.Order.PaymentTradeNo == "" {
		return nil, nil
	}

	prov, err := s.getRefundProvider(ctx, p.Order)
	if err != nil {
		return nil, fmt.Errorf("get refund provider: %w", err)
	}
	if err := validateRefundProviderSnapshotMetadata(p.Order, prov.ProviderKey(), providerMerchantIdentityMetadata(prov)); err != nil {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_PROVIDER_METADATA_MISMATCH", "admin", map[string]any{
			"detail": err.Error(),
		})
		return nil, err
	}
	if preflight, ok := prov.(payment.RefundPreflightProvider); ok {
		if err := preflight.PrepareRefund(ctx); err != nil {
			return nil, fmt.Errorf("prepare refund provider: %w", err)
		}
	}
	return prov, nil
}

func (s *PaymentService) dispatchRefund(ctx context.Context, p *RefundPlan, prov payment.Provider) (*payment.RefundResponse, error) {
	if p.Order.PaymentTradeNo == "" {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_NO_TRADE_NO", "admin", map[string]any{"detail": "skipped"})
		return &payment.RefundResponse{Status: payment.ProviderStatusSuccess}, nil
	}
	if prov == nil {
		return nil, errors.New("refund provider is unavailable")
	}
	callCtx, cancel := context.WithTimeout(ctx, refundProviderCallTimeout)
	defer cancel()
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := prov.Refund(callCtx, payment.RefundRequest{
		TradeNo:           p.Order.PaymentTradeNo,
		OrderID:           p.Order.OutTradeNo,
		ProviderRequestID: p.ProviderRequestID,
		Amount:            formatGatewayRefundAmount(p.GatewayAmount, p.Order),
		Reason:            p.Reason,
	})
	finishProviderCall()
	if err != nil {
		if resp != nil && strings.TrimSpace(resp.Status) == payment.ProviderStatusPending {
			return resp, nil
		}
		return resp, err
	}
	if err := validateRefundProviderResponse(resp); err != nil {
		return resp, err
	}
	return resp, nil
}

func formatGatewayRefundAmount(amount float64, order *dbent.PaymentOrder) string {
	return payment.FormatAmountForCurrency(amount, PaymentOrderCurrency(order))
}

func validateRefundProviderResponse(resp *payment.RefundResponse) error {
	if resp == nil {
		return fmt.Errorf("payment refund response missing")
	}
	status := strings.TrimSpace(resp.Status)
	switch status {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded, payment.ProviderStatusPending:
		return nil
	case payment.ProviderStatusFailed:
		return fmt.Errorf("payment refund failed: status %s", status)
	default:
		return fmt.Errorf("payment refund returned unknown status: %s", status)
	}
}

func (s *PaymentService) QueryAndFinalizeRefund(ctx context.Context, oid int64) (*RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusRefundPending && o.Status != OrderStatusRefunding {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only refunding or refund pending orders can be finalized")
	}

	prov, err := s.getRefundProvider(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("get refund provider: %w", err)
	}
	if err := validateRefundProviderSnapshotMetadata(o, prov.ProviderKey(), providerMerchantIdentityMetadata(prov)); err != nil {
		s.writeAuditLog(ctx, o.ID, "REFUND_PROVIDER_METADATA_MISMATCH", "admin", map[string]any{
			"detail": err.Error(),
		})
		return nil, err
	}
	pendingDetail, err := s.latestRefundPendingDetail(ctx, oid)
	if err != nil {
		return nil, err
	}
	if o.Status == OrderStatusRefunding {
		return s.recoverRefundingAttempt(ctx, prov, o, pendingDetail)
	}
	if prov.ProviderKey() == payment.TypeAlipay && pendingDetail.ProviderRequestID == "" {
		return nil, refundReconciliationRequired(payment.ErrRefundRequestIDMissing)
	}
	queryProvider, ok := prov.(payment.RefundQueryProvider)
	if !ok {
		return nil, infraerrors.BadRequest("REFUND_QUERY_UNSUPPORTED", "this payment provider does not support refund status query; please verify manually")
	}
	callCtx, cancel := context.WithTimeout(ctx, refundProviderCallTimeout)
	defer cancel()
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	plan := s.refundFinalizePlan(o, pendingDetail)
	resp, err := queryProvider.QueryRefund(callCtx, payment.RefundQueryRequest{
		TradeNo:           o.PaymentTradeNo,
		OrderID:           o.OutTradeNo,
		ProviderRequestID: pendingDetail.ProviderRequestID,
		RefundID:          pendingDetail.RefundID,
		Amount:            formatGatewayRefundAmount(plan.GatewayAmount, o),
	})
	finishProviderCall()
	if err != nil {
		return nil, fmt.Errorf("query refund: %w", err)
	}
	if err := validateRefundProviderResponse(resp); err != nil {
		if refundID := refundResponseID(resp); refundID != "" {
			pendingDetail.RefundID = refundID
		}
		return s.finalizeRefundFailed(ctx, o, pendingDetail, err)
	}

	if pendingDetail.DeductionRollbackOK && plan.DeductBalance && o.OrderType == payment.OrderTypeSubscription && plan.SubscriptionID == 0 {
		if err := s.ensureRefundPlanSubscriptionID(ctx, o, plan); err != nil {
			return nil, err
		}
	}
	switch strings.TrimSpace(resp.Status) {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded:
		if refundID := refundResponseID(resp); refundID != "" {
			plan.RefundID = refundID
		}
		if !pendingDetail.DeductionRollbackOK {
			plan.DeductionApplied = true
			plan.BalanceToDeduct = pendingDetail.BalanceToDeduct
			plan.SubDaysToDeduct = pendingDetail.SubDaysToDeduct
		}
		return s.finalizePendingRefundSuccess(ctx, plan)
	case payment.ProviderStatusPending:
		if !pendingDetail.DeductionRollbackOK {
			return s.rollbackPendingRefundDeduction(ctx, plan, pendingDetail, nil)
		}
		s.writeAuditLog(ctx, oid, "REFUND_QUERY_PENDING", "admin", map[string]any{"refundID": resp.RefundID})
		return &RefundResult{Success: false, Warning: "gateway refund is still pending confirmation"}, nil
	default:
		return s.finalizeRefundFailed(ctx, o, pendingDetail, fmt.Errorf("payment refund returned unknown status: %s", strings.TrimSpace(resp.Status)))
	}
}

func (s *PaymentService) recoverRefundingAttempt(ctx context.Context, prov payment.Provider, o *dbent.PaymentOrder, detail refundPendingAuditDetail) (*RefundResult, error) {
	if !detail.HasPendingAudit || detail.Phase != "dispatching" {
		return nil, infraerrors.Conflict("REFUND_RECONCILIATION_REQUIRED", "refund attempt context is incomplete; verify the provider result manually")
	}
	if prov.ProviderKey() == payment.TypeAlipay && detail.ProviderRequestID == "" {
		return nil, refundReconciliationRequired(payment.ErrRefundRequestIDMissing)
	}
	queryProvider, canQuery := prov.(payment.RefundQueryProvider)
	if !canQuery && !refundReplaySafe(prov.ProviderKey()) {
		return nil, infraerrors.Conflict("REFUND_RECONCILIATION_REQUIRED", "this payment provider cannot safely replay an interrupted refund; verify it manually")
	}
	claimedDetail, err := s.claimRefundRecovery(ctx, o.ID, detail.AttemptID)
	if err != nil {
		return nil, err
	}
	detail = claimedDetail

	plan := s.refundFinalizePlan(o, detail)
	plan.PreviousStatus = detail.PreviousStatus
	plan.DeductionApplied = detail.DeductionApplied
	plan.BalanceToDeduct = detail.BalanceToDeduct
	plan.SubDaysToDeduct = detail.SubDaysToDeduct
	if canQuery {
		queryCtx, queryCancel := context.WithTimeout(ctx, refundRecoveryQueryTimeout)
		finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
		resp, err := queryProvider.QueryRefund(queryCtx, payment.RefundQueryRequest{
			TradeNo:           o.PaymentTradeNo,
			OrderID:           o.OutTradeNo,
			ProviderRequestID: plan.ProviderRequestID,
			RefundID:          detail.RefundID,
			Amount:            formatGatewayRefundAmount(plan.GatewayAmount, o),
		})
		finishProviderCall()
		queryCancel()
		if err == nil {
			err = validateRefundProviderResponse(resp)
		}
		if err == nil || resp != nil {
			return s.settleRefundProviderOutcome(ctx, plan, resp, err)
		}
	}
	if !refundReplaySafe(prov.ProviderKey()) {
		return nil, infraerrors.Conflict("REFUND_RECONCILIATION_REQUIRED", "this payment provider cannot safely replay an interrupted refund; verify it manually")
	}
	replayCtx, replayCancel := context.WithTimeout(ctx, refundRecoveryReplayTimeout)
	defer replayCancel()
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := prov.Refund(replayCtx, payment.RefundRequest{
		TradeNo:           o.PaymentTradeNo,
		OrderID:           o.OutTradeNo,
		ProviderRequestID: plan.ProviderRequestID,
		Amount:            formatGatewayRefundAmount(plan.GatewayAmount, o),
		Reason:            plan.Reason,
	})
	finishProviderCall()
	if err == nil {
		err = validateRefundProviderResponse(resp)
	}
	return s.settleRefundProviderOutcome(ctx, plan, resp, err)
}

func refundReconciliationRequired(cause error) error {
	return infraerrors.Conflict("REFUND_RECONCILIATION_REQUIRED", "refund request identity is unavailable; verify the provider result manually").WithCause(cause)
}

func (s *PaymentService) claimRefundRecovery(ctx context.Context, orderID int64, expectedAttemptID string) (_ refundPendingAuditDetail, err error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("begin refund recovery claim: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)
	claimed, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(orderID), paymentorder.StatusEQ(OrderStatusRefunding)).
		SetStatus(OrderStatusRefunding).
		Save(txCtx)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("claim refund recovery order: %w", err)
	}
	if claimed == 0 {
		return refundPendingAuditDetail{}, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	logEntry, err := tx.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Only(txCtx)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("load refund recovery attempt: %w", err)
	}
	detail, err := decodeRefundPendingAuditDetail([]byte(logEntry.Detail))
	if err != nil {
		return refundPendingAuditDetail{}, err
	}
	if detail.AttemptID != strings.TrimSpace(expectedAttemptID) {
		return refundPendingAuditDetail{}, infraerrors.Conflict("CONFLICT", "refund attempt changed")
	}
	now := time.Now().UTC()
	if detail.AttemptID != "" && !detail.AttemptStartedAt.IsZero() && now.Before(detail.AttemptStartedAt.Add(refundRecoveryDelay)) {
		return refundPendingAuditDetail{}, infraerrors.Conflict("REFUND_IN_PROGRESS", "refund provider call may still be in progress; retry reconciliation later")
	}
	detail.AttemptID = uuid.NewString()
	detail.AttemptStartedAt = now
	var raw map[string]any
	if err := json.Unmarshal([]byte(logEntry.Detail), &raw); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("decode refund recovery audit: %w", err)
	}
	raw["attemptID"] = detail.AttemptID
	raw["attemptStartedAt"] = detail.AttemptStartedAt
	detailJSON, err := json.Marshal(raw)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("encode refund recovery audit: %w", err)
	}
	if _, err := tx.PaymentAuditLog.UpdateOneID(logEntry.ID).SetDetail(string(detailJSON)).Save(txCtx); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("persist refund recovery claim: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("commit refund recovery claim: %w", err)
	}
	tx = nil
	return detail, nil
}

func refundReplaySafe(providerKey string) bool {
	switch strings.TrimSpace(providerKey) {
	case payment.TypeStripe, payment.TypeAlipay, payment.TypeWxpay, payment.TypeAirwallex:
		return true
	default:
		return false
	}
}

func (s *PaymentService) settleRefundProviderOutcome(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse, providerErr error) (*RefundResult, error) {
	status := ""
	if resp != nil {
		status = strings.TrimSpace(resp.Status)
	}
	if !psSliceContains([]string{payment.ProviderStatusSuccess, payment.ProviderStatusRefunded, payment.ProviderStatusPending, payment.ProviderStatusFailed}, status) {
		detail := refundAttemptAuditDetail(p, refundResponseID(resp), false, "dispatching")
		if providerErr != nil {
			detail["providerError"] = psErrMsg(providerErr)
		}
		if err := s.persistUnresolvedRefundProviderOutcome(ctx, p, detail); err != nil {
			return nil, err
		}
		return nil, infraerrors.InternalServer("REFUND_RECONCILIATION_REQUIRED", "refund provider result is unknown; verify or query it before retrying")
	}

	detail, err := s.recordRefundProviderOutcome(ctx, p, resp, providerErr)
	if err != nil {
		return nil, err
	}

	switch status {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded:
		plan := s.refundFinalizePlan(p.Order, detail)
		plan.PreviousStatus = p.PreviousStatus
		plan.DeductionApplied = p.DeductionApplied || detail.DeductionApplied
		if plan.DeductionApplied {
			plan.BalanceToDeduct = p.BalanceToDeduct
			plan.SubDaysToDeduct = p.SubDaysToDeduct
			plan.SubscriptionID = p.SubscriptionID
			plan.SubscriptionRevoked = p.SubscriptionRevoked
		}
		return s.finalizePendingRefundSuccess(ctx, plan)
	case payment.ProviderStatusPending:
		return s.rollbackPendingRefundDeduction(ctx, p, detail, providerErr)
	default:
		if providerErr == nil {
			providerErr = fmt.Errorf("payment refund returned unknown status: %s", status)
		}
		return s.finalizeRefundFailed(ctx, p.Order, detail, providerErr)
	}
}

func (s *PaymentService) persistUnresolvedRefundProviderOutcome(ctx context.Context, p *RefundPlan, detail map[string]any) (err error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin unresolved refund outcome: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)
	if _, err = s.claimRefundAttemptState(txCtx, tx.Client(), p.OrderID, OrderStatusRefunding, OrderStatusRefunding, p.AttemptID); err != nil {
		return err
	}
	if err = s.upsertRefundAudit(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", detail); err != nil {
		return fmt.Errorf("persist unresolved refund provider result: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit unresolved refund provider result: %w", err)
	}
	tx = nil
	return nil
}

func (s *PaymentService) recordRefundProviderOutcome(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse, providerErr error) (refundPendingAuditDetail, error) {
	detail := refundAttemptAuditDetail(p, refundResponseID(resp), false, "provider_confirmed")
	if resp != nil {
		detail["providerStatus"] = strings.TrimSpace(resp.Status)
	}
	if providerErr != nil {
		detail["providerError"] = psErrMsg(providerErr)
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("marshal refund provider outcome: %w", err)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("begin refund provider outcome: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if _, err := s.claimRefundAttemptState(txCtx, tx.Client(), p.OrderID, OrderStatusRefunding, OrderStatusRefundPending, p.AttemptID); err != nil {
		return refundPendingAuditDetail{}, err
	}
	if _, err := tx.PaymentOrder.UpdateOneID(p.OrderID).
		SetRefundAmount(p.RefundAmount).
		SetRefundReason(p.Reason).
		SetForceRefund(p.Force).
		Save(txCtx); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("persist refund provider outcome: %w", err)
	}
	if err := s.upsertRefundAuditJSON(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", detailJSON); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("persist refund provider outcome: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("commit refund provider outcome: %w", err)
	}

	parsed, err := decodeRefundPendingAuditDetail(detailJSON)
	if err != nil {
		return refundPendingAuditDetail{}, err
	}
	return parsed, nil
}

func (s *PaymentService) finalizePendingRefundSuccess(ctx context.Context, p *RefundPlan) (_ *RefundResult, err error) {
	observedRefundID := strings.TrimSpace(p.RefundID)
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund finalization: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	latestDetail, err := s.claimRefundAttemptState(txCtx, tx.Client(), p.OrderID, OrderStatusRefundPending, OrderStatusRefunding, p.AttemptID)
	if err != nil {
		return nil, err
	}
	p = s.refundFinalizePlan(p.Order, latestDetail)
	if observedRefundID != "" {
		p.RefundID = observedRefundID
	}
	p.DeductionApplied = !latestDetail.DeductionRollbackOK
	if p.DeductionApplied {
		p.BalanceToDeduct = latestDetail.BalanceToDeduct
		p.SubDaysToDeduct = latestDetail.SubDaysToDeduct
	}

	var mutatedSubscription *UserSubscription
	if !p.DeductionApplied {
		mutatedSubscription, err = s.applyRefundDeduction(txCtx, p)
		if err != nil {
			if errors.Is(err, errRefundForceRequired) {
				return &RefundResult{
					Success:      false,
					Warning:      "gateway refund succeeded, but full balance deduction is unavailable; restore the balance or reconcile manually before finalizing",
					RequireForce: true,
				}, nil
			}
			return nil, err
		}
		if p.DeductBalance {
			p.DeductionApplied = true
		}
	}
	if err := s.resolveRefundRollbackFailure(txCtx, tx.Client(), p.OrderID); err != nil {
		return nil, err
	}
	result, err := s.markRefundOkTx(txCtx, tx.Client(), p)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund finalization: %w", err)
	}
	tx = nil
	s.invalidateRefundSubscriptionCaches(p.OrderID, mutatedSubscription, "final deduction")
	return result, nil
}

func (s *PaymentService) refundFinalizePlan(o *dbent.PaymentOrder, pendingDetail refundPendingAuditDetail) *RefundPlan {
	refundAmount := pendingDetail.RefundAmount
	if refundAmount == 0 {
		refundAmount = o.RefundAmount
	}
	reason := pendingDetail.Reason
	if reason == "" {
		reason = strings.TrimSpace(psStringValue(o.RefundReason))
	}
	if reason == "" {
		reason = fmt.Sprintf("refund order:%d", o.ID)
	}
	plan := &RefundPlan{
		OrderID:                              o.ID,
		Order:                                o,
		AttemptID:                            pendingDetail.AttemptID,
		ProviderRequestID:                    pendingDetail.ProviderRequestID,
		RefundID:                             pendingDetail.RefundID,
		AttemptStartedAt:                     pendingDetail.AttemptStartedAt,
		PreviousStatus:                       pendingDetail.PreviousStatus,
		RefundAmount:                         refundAmount,
		GatewayAmount:                        calculateGatewayRefundAmount(o.Amount, o.PayAmount, refundAmount, PaymentOrderCurrency(o)),
		Reason:                               reason,
		Force:                                pendingDetail.Force || o.ForceRefund,
		DeductBalance:                        pendingDetail.DeductBalance,
		DeductionType:                        pendingDetail.DeductionType,
		SubscriptionID:                       pendingDetail.SubscriptionID,
		SubscriptionRevoked:                  pendingDetail.SubscriptionRevoked,
		SubscriptionExpiresAtBeforeDeduction: pendingDetail.SubscriptionExpiresAtBeforeDeduction,
		SubscriptionExpiresAtAfterDeduction:  pendingDetail.SubscriptionExpiresAtAfterDeduction,
	}
	if !plan.DeductBalance {
		plan.DeductionType = payment.DeductionTypeNone
		return plan
	}
	if plan.DeductionType == "" || plan.DeductionType == payment.DeductionTypeNone {
		plan.DeductionType = payment.DeductionTypeBalance
		if o.OrderType == payment.OrderTypeSubscription {
			plan.DeductionType = payment.DeductionTypeSubscription
		}
	}
	switch plan.DeductionType {
	case payment.DeductionTypeBalance:
		plan.BalanceToDeduct = pendingDetail.BalanceToDeduct
		if plan.BalanceToDeduct == 0 && pendingDetail.DeductionRollbackOK && o.OrderType == payment.OrderTypeBalance && (!pendingDetail.HasPendingAudit || (!pendingDetail.HasDeductBalance && !pendingDetail.HasDeductionType)) {
			plan.BalanceToDeduct = refundAmount
		}
	case payment.DeductionTypeSubscription:
		plan.SubDaysToDeduct = pendingDetail.SubDaysToDeduct
	}
	if !pendingDetail.DeductionRollbackOK {
		plan.BalanceToDeduct = 0
		plan.SubDaysToDeduct = 0
	}
	return plan
}

func (s *PaymentService) ensureRefundPlanSubscriptionID(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan) error {
	if p == nil || p.SubscriptionID > 0 || p.DeductionType != payment.DeductionTypeSubscription {
		return nil
	}
	if s == nil || s.subscriptionSvc == nil {
		return fmt.Errorf("subscription service unavailable")
	}
	if o == nil || o.SubscriptionGroupID == nil {
		return fmt.Errorf("subscription group unavailable")
	}
	sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID)
	if err != nil {
		return err
	}
	if sub == nil || sub.ID == 0 {
		return ErrSubscriptionNotFound
	}
	p.SubscriptionID = sub.ID
	return nil
}

func (s *PaymentService) applyRefundDeduction(ctx context.Context, p *RefundPlan) (*UserSubscription, error) {
	if p.DeductionType == payment.DeductionTypeBalance {
		requested := p.RefundAmount
		deducted, err := s.deductAvailableBalance(ctx, p.Order.UserID, requested)
		if err != nil {
			return nil, fmt.Errorf("deduction: %w", err)
		}
		if !p.Force && requested-deducted > paymentAmountToleranceForCurrency(PaymentOrderCurrency(p.Order)) {
			return nil, errRefundForceRequired
		}
		p.BalanceToDeduct = deducted
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		return s.applyRefundSubscriptionDeduction(ctx, p, true)
	}
	return nil, nil
}

func (s *PaymentService) applyRefundSubscriptionDeduction(ctx context.Context, p *RefundPlan, deferCacheInvalidation bool) (*UserSubscription, error) {
	p.SubscriptionRevoked = false
	p.SubscriptionExpiresAtBeforeDeduction = time.Time{}
	p.SubscriptionExpiresAtAfterDeduction = time.Time{}
	daysToDeduct := p.SubDaysToDeduct
	if daysToDeduct > MaxValidityDays {
		daysToDeduct = MaxValidityDays
	}
	var sub *UserSubscription
	err := s.subscriptionSvc.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		locked, err := s.subscriptionSvc.userSubRepo.GetByIDForUpdate(txCtx, p.SubscriptionID)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if !locked.ExpiresAt.After(now) {
			return infraerrors.BadRequest("CANNOT_SHORTEN_EXPIRED", "cannot shorten an expired subscription")
		}
		newExpiresAt := locked.ExpiresAt.AddDate(0, 0, -daysToDeduct)
		fullyDeducted := !newExpiresAt.After(now)
		if fullyDeducted {
			newExpiresAt = now
		}
		if err := s.subscriptionSvc.userSubRepo.ExtendExpiry(txCtx, locked.ID, newExpiresAt); err != nil {
			return err
		}
		if fullyDeducted && locked.Status != SubscriptionStatusExpired {
			if err := s.subscriptionSvc.userSubRepo.UpdateStatus(txCtx, locked.ID, SubscriptionStatusExpired); err != nil {
				return err
			}
			locked.Status = SubscriptionStatusExpired
		}
		if fullyDeducted {
			p.SubscriptionRevoked = true
			p.SubscriptionExpiresAtBeforeDeduction = locked.ExpiresAt
			p.SubscriptionExpiresAtAfterDeduction = newExpiresAt
		}
		locked.ExpiresAt = newExpiresAt
		sub = locked
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("deduct subscription days: %w", err)
	}
	if p.SubscriptionRevoked {
		slog.Info("subscription deduction exhausted current term", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct)
	}
	if !deferCacheInvalidation {
		s.invalidateRefundSubscriptionCaches(p.OrderID, sub, "deduction")
	}
	return sub, nil
}

func (s *PaymentService) invalidateRefundSubscriptionCaches(orderID int64, sub *UserSubscription, operation string) {
	if sub == nil {
		return
	}
	if err := s.subscriptionSvc.invalidateSubscriptionCaches(sub.UserID, sub.GroupID); err != nil {
		slog.Error("refund subscription cache invalidation failed after database mutation",
			"orderID", orderID,
			"operation", operation,
			"subscriptionID", sub.ID,
			"userID", sub.UserID,
			"groupID", sub.GroupID,
			"error", err,
		)
	}
}

func (s *PaymentService) finalizeRefundFailed(ctx context.Context, o *dbent.PaymentOrder, pendingDetail refundPendingAuditDetail, gErr error) (_ *RefundResult, err error) {
	observedRefundID := strings.TrimSpace(pendingDetail.RefundID)
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund failure finalization: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	latestDetail, err := s.claimRefundAttemptState(txCtx, tx.Client(), o.ID, OrderStatusRefundPending, OrderStatusRefunding, pendingDetail.AttemptID)
	if err != nil {
		return nil, err
	}
	pendingDetail = latestDetail
	if observedRefundID != "" {
		pendingDetail.RefundID = observedRefundID
	}

	failurePlan := s.refundFinalizePlan(o, pendingDetail)
	failurePlan.DeductionApplied = pendingDetail.DeductionApplied
	balanceRolledBack := pendingDetail.BalanceRolledBack
	subDaysRolledBack := pendingDetail.SubDaysRolledBack
	var compensatedSubscription *UserSubscription
	if !pendingDetail.DeductionRollbackOK {
		failurePlan.BalanceToDeduct = pendingDetail.BalanceToDeduct
		failurePlan.SubDaysToDeduct = pendingDetail.SubDaysToDeduct
		failurePlan.DeductionApplied = pendingDetail.DeductionApplied || failurePlan.BalanceToDeduct > 0 || failurePlan.SubDaysToDeduct > 0
		if failurePlan.DeductionType == payment.DeductionTypeSubscription && failurePlan.SubDaysToDeduct > 0 && failurePlan.SubscriptionID == 0 {
			if err := s.ensureRefundPlanSubscriptionID(txCtx, o, failurePlan); err != nil {
				warning := "refund failed but subscription rollback context is unavailable: " + psErrMsg(err)
				_ = tx.Rollback()
				tx = nil
				s.writeAuditLog(ctx, o.ID, "REFUND_ROLLBACK_CONTEXT_MISSING", "admin", map[string]any{"detail": warning})
				return nil, infraerrors.InternalServer("REFUND_ROLLBACK_CONTEXT_MISSING", warning)
			}
		}
		mutatedSubscription, rollbackErr := s.rollbackRefund(txCtx, failurePlan, true)
		if rollbackErr != nil {
			warning := "refund failed but deduction rollback is still pending: " + psErrMsg(gErr)
			_ = tx.Rollback()
			tx = nil
			if auditErr := s.recordRefundRollbackFailure(ctx, failurePlan, gErr, rollbackErr); auditErr != nil {
				return nil, auditErr
			}
			s.writeAuditLog(ctx, o.ID, "REFUND_ROLLBACK_STILL_PENDING", "admin", map[string]any{"detail": warning})
			return nil, infraerrors.InternalServer("REFUND_ROLLBACK_PENDING", warning)
		}
		compensatedSubscription = mutatedSubscription
		balanceRolledBack = failurePlan.BalanceToDeduct
		subDaysRolledBack = failurePlan.SubDaysToDeduct
		if err := s.resolveRefundRollbackFailure(txCtx, tx.Client(), o.ID); err != nil {
			return nil, err
		}
	}
	failedDetail := refundAttemptAuditDetail(failurePlan, pendingDetail.RefundID, true, "provider_confirmed")
	failedDetail["providerStatus"] = payment.ProviderStatusFailed
	failedDetail["providerError"] = psErrMsg(gErr)
	failedDetail["balanceRolledBack"] = balanceRolledBack
	failedDetail["subDaysRolledBack"] = subDaysRolledBack
	if err := s.upsertRefundAudit(txCtx, tx.Client(), o.ID, "REFUND_PENDING", failedDetail); err != nil {
		return nil, fmt.Errorf("persist refund failure audit: %w", err)
	}
	now := time.Now()
	if _, err = tx.PaymentOrder.UpdateOneID(o.ID).
		SetStatus(OrderStatusRefundFailed).
		SetFailedAt(now).
		SetFailedReason(psErrMsg(gErr)).
		Save(txCtx); err != nil {
		return nil, fmt.Errorf("mark refund failed: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund failure finalization: %w", err)
	}
	tx = nil
	s.invalidateRefundSubscriptionCaches(o.ID, compensatedSubscription, "failure compensation")
	s.writeAuditLog(ctx, o.ID, "REFUND_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
	return &RefundResult{Success: false, Warning: "gateway refund failed: " + psErrMsg(gErr)}, nil
}

type refundPendingAuditDetail struct {
	HasPendingAudit                      bool      `json:"-"`
	HasDeductBalance                     bool      `json:"-"`
	HasDeductionType                     bool      `json:"-"`
	AttemptID                            string    `json:"attemptID"`
	ProviderRequestID                    string    `json:"providerRequestID"`
	AttemptStartedAt                     time.Time `json:"attemptStartedAt"`
	Phase                                string    `json:"phase"`
	PreviousStatus                       string    `json:"previousStatus"`
	RefundID                             string    `json:"refundID"`
	RefundAmount                         float64   `json:"refundAmount"`
	Reason                               string    `json:"reason"`
	Force                                bool      `json:"force"`
	DeductBalance                        bool      `json:"deductBalance"`
	DeductionApplied                     bool      `json:"deductionApplied"`
	DeductionType                        string    `json:"deductionType"`
	BalanceToDeduct                      float64   `json:"balanceToDeduct"`
	BalanceDeducted                      float64   `json:"balanceDeducted"`
	BalanceRolledBack                    float64   `json:"balanceRolledBack"`
	SubDaysToDeduct                      int       `json:"subDaysToDeduct"`
	SubDaysDeducted                      int       `json:"subDaysDeducted"`
	SubDaysRolledBack                    int       `json:"subDaysRolledBack"`
	SubscriptionID                       int64     `json:"subscriptionID"`
	SubscriptionRevoked                  bool      `json:"subscriptionRevoked"`
	SubscriptionExpiresAtBeforeDeduction time.Time `json:"subscriptionExpiresAtBeforeDeduction"`
	SubscriptionExpiresAtAfterDeduction  time.Time `json:"subscriptionExpiresAtAfterDeduction"`
	DeductionRollbackOK                  bool      `json:"deductionRollbackOK"`
}

func (s *PaymentService) latestRefundPendingDetail(ctx context.Context, oid int64) (refundPendingAuditDetail, error) {
	return latestRefundPendingDetailWithClient(ctx, s.entClient, oid)
}

func latestRefundPendingDetailWithClient(ctx context.Context, client *dbent.Client, oid int64) (refundPendingAuditDetail, error) {
	logEntry, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Order(paymentauditlog.ByCreatedAt(sql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return refundPendingAuditDetail{DeductBalance: true, DeductionRollbackOK: true}, nil
		}
		return refundPendingAuditDetail{}, fmt.Errorf("load refund pending audit: %w", err)
	}
	if logEntry == nil {
		return refundPendingAuditDetail{}, fmt.Errorf("load refund pending audit: empty result")
	}
	detail, err := decodeRefundPendingAuditDetail([]byte(logEntry.Detail))
	if err != nil {
		return refundPendingAuditDetail{}, err
	}
	return detail, nil
}

func decodeRefundPendingAuditDetail(detailJSON []byte) (refundPendingAuditDetail, error) {
	detail := refundPendingAuditDetail{HasPendingAudit: true, DeductionRollbackOK: true}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(detailJSON, &raw); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("decode refund pending audit fields: %w", err)
	}
	if raw == nil {
		return refundPendingAuditDetail{}, fmt.Errorf("decode refund pending audit fields: expected object")
	}
	_, detail.HasDeductBalance = raw["deductBalance"]
	_, detail.HasDeductionType = raw["deductionType"]
	if err := json.Unmarshal(detailJSON, &detail); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("decode refund pending audit detail: %w", err)
	}
	detail.HasPendingAudit = true
	detail.AttemptID = strings.TrimSpace(detail.AttemptID)
	detail.ProviderRequestID = strings.TrimSpace(detail.ProviderRequestID)
	detail.Phase = strings.TrimSpace(detail.Phase)
	detail.PreviousStatus = strings.TrimSpace(detail.PreviousStatus)
	detail.RefundID = strings.TrimSpace(detail.RefundID)
	detail.Reason = strings.TrimSpace(detail.Reason)
	detail.DeductionType = strings.TrimSpace(detail.DeductionType)
	if !detail.HasDeductBalance && !detail.HasDeductionType {
		detail.DeductBalance = true
	}
	if detail.BalanceToDeduct == 0 {
		if detail.BalanceDeducted != 0 {
			detail.BalanceToDeduct = detail.BalanceDeducted
		} else if detail.BalanceRolledBack != 0 {
			detail.BalanceToDeduct = detail.BalanceRolledBack
		}
	}
	if detail.SubDaysToDeduct == 0 {
		if detail.SubDaysDeducted != 0 {
			detail.SubDaysToDeduct = detail.SubDaysDeducted
		} else if detail.SubDaysRolledBack != 0 {
			detail.SubDaysToDeduct = detail.SubDaysRolledBack
		}
	}
	return detail, nil
}

func refundAttemptAuditDetail(p *RefundPlan, refundID string, rollbackOK bool, phase string) map[string]any {
	deductBalance := p.DeductBalance || p.DeductionType != "" && p.DeductionType != payment.DeductionTypeNone
	detail := map[string]any{
		"attemptID":           p.AttemptID,
		"providerRequestID":   p.ProviderRequestID,
		"attemptStartedAt":    p.AttemptStartedAt,
		"phase":               phase,
		"previousStatus":      p.PreviousStatus,
		"refundID":            strings.TrimSpace(refundID),
		"refundAmount":        p.RefundAmount,
		"reason":              p.Reason,
		"force":               p.Force,
		"deductBalance":       deductBalance,
		"deductionApplied":    p.DeductionApplied,
		"deductionType":       p.DeductionType,
		"balanceToDeduct":     p.BalanceToDeduct,
		"subDaysToDeduct":     p.SubDaysToDeduct,
		"subscriptionID":      p.SubscriptionID,
		"subscriptionRevoked": p.SubscriptionRevoked,
		"deductionRollbackOK": rollbackOK,
	}
	if !p.SubscriptionExpiresAtBeforeDeduction.IsZero() {
		detail["subscriptionExpiresAtBeforeDeduction"] = p.SubscriptionExpiresAtBeforeDeduction
	}
	if !p.SubscriptionExpiresAtAfterDeduction.IsZero() {
		detail["subscriptionExpiresAtAfterDeduction"] = p.SubscriptionExpiresAtAfterDeduction
	}
	return detail
}

func (s *PaymentService) claimRefundAttemptState(ctx context.Context, client *dbent.Client, orderID int64, currentStatus, nextStatus, attemptID string) (refundPendingAuditDetail, error) {
	claimed, err := client.PaymentOrder.Update().
		Where(paymentorder.IDEQ(orderID), paymentorder.StatusEQ(currentStatus)).
		SetStatus(nextStatus).
		Save(ctx)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("claim refund attempt state: %w", err)
	}
	if claimed == 0 {
		return refundPendingAuditDetail{}, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	detail, err := latestRefundPendingDetailWithClient(ctx, client, orderID)
	if err != nil {
		return refundPendingAuditDetail{}, err
	}
	if detail.AttemptID != strings.TrimSpace(attemptID) {
		return refundPendingAuditDetail{}, infraerrors.Conflict("CONFLICT", "refund attempt changed")
	}
	return detail, nil
}

func (s *PaymentService) upsertRefundAudit(ctx context.Context, client *dbent.Client, orderID int64, action string, detail map[string]any) error {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal %s audit: %w", action, err)
	}
	return s.upsertRefundAuditJSON(ctx, client, orderID, action, detailJSON)
}

func (s *PaymentService) upsertRefundAuditJSON(ctx context.Context, client *dbent.Client, orderID int64, action string, detailJSON []byte) error {
	return client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(orderID, 10)).
		SetAction(action).
		SetDetail(string(detailJSON)).
		SetOperator("admin").
		OnConflict(
			sql.ConflictColumns(paymentauditlog.FieldOrderID, paymentauditlog.FieldAction),
			sql.ConflictWhere(sql.ExprP("action IN ('REFUND_PENDING', 'REFUND_ROLLBACK_FAILED')")),
		).
		UpdateNewValues().
		Exec(ctx)
}

func (s *PaymentService) ensureRefundRetryAllowed(ctx context.Context, orderID int64) error {
	logEntry, err := s.entClient.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)),
			paymentauditlog.ActionEQ("REFUND_ROLLBACK_FAILED"),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("check refund rollback state: %w", err)
	}
	var detail struct {
		Resolved bool `json:"resolved"`
	}
	if err := json.Unmarshal([]byte(logEntry.Detail), &detail); err != nil {
		return fmt.Errorf("decode refund rollback state: %w", err)
	}
	if !detail.Resolved {
		return infraerrors.Conflict("REFUND_ROLLBACK_PENDING", "refund deduction rollback requires manual reconciliation")
	}
	return nil
}

func (s *PaymentService) resolveRefundRollbackFailure(ctx context.Context, client *dbent.Client, orderID int64) error {
	logEntry, err := client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)),
			paymentauditlog.ActionEQ("REFUND_ROLLBACK_FAILED"),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("load refund rollback failure: %w", err)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(logEntry.Detail), &detail); err != nil {
		return fmt.Errorf("decode refund rollback failure: %w", err)
	}
	if detail == nil {
		return fmt.Errorf("decode refund rollback failure: expected object")
	}
	detail["resolved"] = true
	detail["resolvedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("encode resolved refund rollback failure: %w", err)
	}
	if _, err := client.PaymentAuditLog.UpdateOneID(logEntry.ID).SetDetail(string(detailJSON)).SetOperator("admin").Save(ctx); err != nil {
		return fmt.Errorf("resolve refund rollback failure: %w", err)
	}
	return nil
}

// getRefundProvider creates a provider using the order's original instance config.
// Delegates to getOrderProvider which handles instance lookup and fallback.
func (s *PaymentService) getRefundProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("refund provider instance is unavailable for order %d", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
}

func (s *PaymentService) markRefundOkTx(ctx context.Context, client *dbent.Client, p *RefundPlan) (*RefundResult, error) {
	fs := OrderStatusRefunded
	if p.RefundAmount < p.Order.Amount {
		fs = OrderStatusPartiallyRefunded
	}
	now := time.Now()
	_, err := client.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(fs).SetRefundAmount(p.RefundAmount).SetRefundReason(p.Reason).SetRefundAt(now).SetForceRefund(p.Force).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark refund: %w", err)
	}
	auditDetail := refundAttemptAuditDetail(p, p.RefundID, true, "completed")
	auditDetail["balanceDeducted"] = p.BalanceToDeduct
	auditDetail["subDaysDeducted"] = p.SubDaysToDeduct
	detail, err := json.Marshal(auditDetail)
	if err != nil {
		return nil, fmt.Errorf("marshal refund audit: %w", err)
	}
	if _, err := client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(p.OrderID, 10)).
		SetAction("REFUND_SUCCESS").
		SetDetail(string(detail)).
		SetOperator("admin").
		Save(ctx); err != nil {
		return nil, fmt.Errorf("write refund audit: %w", err)
	}
	if _, err := client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(p.OrderID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("clear refund attempt audit: %w", err)
	}
	return &RefundResult{Success: true, BalanceDeducted: p.BalanceToDeduct, SubDaysDeducted: p.SubDaysToDeduct}, nil
}

func (s *PaymentService) rollbackPendingRefundDeduction(ctx context.Context, p *RefundPlan, detail refundPendingAuditDetail, providerErr error) (_ *RefundResult, err error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund pending rollback: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	latestDetail, err := s.claimRefundAttemptState(txCtx, tx.Client(), p.OrderID, OrderStatusRefundPending, OrderStatusRefunding, p.AttemptID)
	if err != nil {
		return nil, err
	}
	if latestDetail.DeductionRollbackOK {
		if _, err := tx.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(OrderStatusRefundPending).Save(txCtx); err != nil {
			return nil, fmt.Errorf("restore refund pending status: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit refund pending status: %w", err)
		}
		tx = nil
		return &RefundResult{Success: false, Warning: "gateway refund is pending confirmation"}, nil
	}

	detail = latestDetail
	compensationPlan := s.refundFinalizePlan(p.Order, latestDetail)
	compensationPlan.PreviousStatus = p.PreviousStatus
	compensationPlan.BalanceToDeduct = detail.BalanceToDeduct
	compensationPlan.SubDaysToDeduct = detail.SubDaysToDeduct
	mutatedSubscription, rollbackErr := s.rollbackRefund(txCtx, compensationPlan, true)
	if rollbackErr != nil {
		auditDetail := refundAttemptAuditDetail(compensationPlan, detail.RefundID, false, "provider_confirmed")
		auditDetail["providerStatus"] = payment.ProviderStatusPending
		if providerErr != nil {
			auditDetail["providerError"] = psErrMsg(providerErr)
		}
		_ = tx.Rollback()
		tx = nil
		if err := s.persistRefundRollbackFailure(ctx, compensationPlan, auditDetail, providerErr, rollbackErr); err != nil {
			return nil, err
		}
		return &RefundResult{Success: false, Warning: "gateway refund is pending confirmation; refund deduction rollback failed"}, nil
	}
	auditDetail := refundAttemptAuditDetail(compensationPlan, detail.RefundID, true, "provider_confirmed")
	auditDetail["providerStatus"] = payment.ProviderStatusPending
	if providerErr != nil {
		auditDetail["providerError"] = psErrMsg(providerErr)
	}
	if err := s.upsertRefundAudit(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", auditDetail); err != nil {
		return nil, fmt.Errorf("persist refund pending rollback: %w", err)
	}
	if err := s.resolveRefundRollbackFailure(txCtx, tx.Client(), p.OrderID); err != nil {
		return nil, err
	}
	if _, err := tx.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(OrderStatusRefundPending).Save(txCtx); err != nil {
		return nil, fmt.Errorf("restore refund pending status: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund pending rollback: %w", err)
	}
	tx = nil
	s.invalidateRefundSubscriptionCaches(p.OrderID, mutatedSubscription, "pending rollback")
	p.BalanceToDeduct = 0
	p.SubDaysToDeduct = 0

	return &RefundResult{Success: false, Warning: "gateway refund is pending confirmation"}, nil
}

func (s *PaymentService) persistRefundRollbackFailure(ctx context.Context, p *RefundPlan, pendingDetail map[string]any, providerErr, rollbackErr error) (err error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin refund rollback failure audit: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)
	if _, err := s.claimRefundAttemptState(txCtx, tx.Client(), p.OrderID, OrderStatusRefundPending, OrderStatusRefundPending, p.AttemptID); err != nil {
		return err
	}
	if err := s.upsertRefundAudit(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", pendingDetail); err != nil {
		return fmt.Errorf("persist unresolved refund rollback: %w", err)
	}
	if err := s.upsertRefundAudit(txCtx, tx.Client(), p.OrderID, "REFUND_ROLLBACK_FAILED", refundRollbackFailureDetail(p, providerErr, rollbackErr)); err != nil {
		return fmt.Errorf("persist refund rollback failure marker: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit refund rollback failure audit: %w", err)
	}
	tx = nil
	logRefundRollbackFailure(p, rollbackErr)
	return nil
}

func refundResponseID(resp *payment.RefundResponse) string {
	if resp == nil {
		return ""
	}
	return strings.TrimSpace(resp.RefundID)
}

func (s *PaymentService) rollbackRefund(ctx context.Context, p *RefundPlan, deferCacheInvalidation bool) (*UserSubscription, error) {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		if _, err := s.userRepo.AdjustBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			return nil, err
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if p.SubscriptionRevoked {
			if p.SubscriptionExpiresAtBeforeDeduction.After(p.SubscriptionExpiresAtAfterDeduction) {
				sub, err := s.subscriptionSvc.restoreRefundSubscriptionTerm(
					ctx,
					p.SubscriptionID,
					p.SubscriptionExpiresAtBeforeDeduction,
					p.SubscriptionExpiresAtAfterDeduction,
				)
				if err != nil {
					return nil, err
				}
				if !deferCacheInvalidation {
					s.invalidateRefundSubscriptionCaches(p.OrderID, sub, "rollback")
				}
				return sub, nil
			}
			sub, err := s.subscriptionSvc.restoreSubscription(ctx, p.SubscriptionID, true)
			if err != nil {
				return nil, err
			}
			if !deferCacheInvalidation {
				s.invalidateRefundSubscriptionCaches(p.OrderID, sub, "rollback")
			}
			return sub, nil
		}
		sub, err := s.subscriptionSvc.extendSubscription(ctx, p.SubscriptionID, p.SubDaysToDeduct, true)
		if err == nil {
			if !deferCacheInvalidation {
				s.invalidateRefundSubscriptionCaches(p.OrderID, sub, "rollback")
			}
			return sub, nil
		}
		if !errors.Is(err, ErrSubscriptionNotFound) {
			return nil, err
		}
		legacySub, lookupErr := s.subscriptionSvc.userSubRepo.GetByIDIncludeDeleted(ctx, p.SubscriptionID)
		if lookupErr != nil {
			return nil, fmt.Errorf("lookup legacy revoked subscription: %w", lookupErr)
		}
		if legacySub == nil || legacySub.DeletedAt == nil {
			return nil, err
		}
		p.SubscriptionRevoked = true
		sub, err = s.subscriptionSvc.restoreSubscription(ctx, p.SubscriptionID, true)
		if err != nil {
			return nil, err
		}
		if !deferCacheInvalidation {
			s.invalidateRefundSubscriptionCaches(p.OrderID, sub, "rollback")
		}
		return sub, nil
	}
	return nil, nil
}

func (s *PaymentService) recordRefundRollbackFailure(ctx context.Context, p *RefundPlan, gErr, rollbackErr error) (err error) {
	observedRefundID := strings.TrimSpace(p.RefundID)
	observedSubscriptionID := p.SubscriptionID
	observedSubscriptionRevoked := p.SubscriptionRevoked
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin refund rollback failure audit: %w", err)
	}
	defer func() {
		if err != nil && tx != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)
	latestDetail, err := s.claimRefundAttemptState(txCtx, tx.Client(), p.OrderID, OrderStatusRefundPending, OrderStatusRefundPending, p.AttemptID)
	if err != nil {
		return err
	}
	p = s.refundFinalizePlan(p.Order, latestDetail)
	if observedRefundID != "" {
		p.RefundID = observedRefundID
	}
	if observedSubscriptionID > 0 {
		p.SubscriptionID = observedSubscriptionID
	}
	p.SubscriptionRevoked = p.SubscriptionRevoked || observedSubscriptionRevoked
	p.BalanceToDeduct = latestDetail.BalanceToDeduct
	p.SubDaysToDeduct = latestDetail.SubDaysToDeduct
	p.DeductionApplied = latestDetail.DeductionApplied || p.BalanceToDeduct > 0 || p.SubDaysToDeduct > 0
	pendingDetail := refundAttemptAuditDetail(p, p.RefundID, false, "provider_confirmed")
	pendingDetail["providerStatus"] = payment.ProviderStatusFailed
	pendingDetail["providerError"] = psErrMsg(gErr)
	pendingDetail["balanceRolledBack"] = latestDetail.BalanceRolledBack
	pendingDetail["subDaysRolledBack"] = latestDetail.SubDaysRolledBack
	if err = s.upsertRefundAudit(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", pendingDetail); err != nil {
		return fmt.Errorf("persist unresolved refund failure: %w", err)
	}
	detail := refundRollbackFailureDetail(p, gErr, rollbackErr)
	if err = s.upsertRefundAudit(txCtx, tx.Client(), p.OrderID, "REFUND_ROLLBACK_FAILED", detail); err != nil {
		return fmt.Errorf("persist refund rollback failure marker: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit refund rollback failure audit: %w", err)
	}
	tx = nil
	logRefundRollbackFailure(p, rollbackErr)
	return nil
}

func refundRollbackFailureDetail(p *RefundPlan, gErr, rollbackErr error) map[string]any {
	detail := map[string]any{"attemptID": p.AttemptID, "gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(rollbackErr), "resolved": false}
	if p.DeductionType == payment.DeductionTypeSubscription {
		detail["subDaysDeducted"] = p.SubDaysToDeduct
		detail["subscriptionRevoked"] = p.SubscriptionRevoked
		if !p.SubscriptionExpiresAtBeforeDeduction.IsZero() {
			detail["subscriptionExpiresAtBeforeDeduction"] = p.SubscriptionExpiresAtBeforeDeduction
		}
		if !p.SubscriptionExpiresAtAfterDeduction.IsZero() {
			detail["subscriptionExpiresAtAfterDeduction"] = p.SubscriptionExpiresAtAfterDeduction
		}
	} else {
		detail["balanceDeducted"] = p.BalanceToDeduct
	}
	return detail
}

func logRefundRollbackFailure(p *RefundPlan, rollbackErr error) {
	if p.DeductionType == payment.DeductionTypeSubscription {
		slog.Error("[CRITICAL] subscription rollback failed", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct, "error", rollbackErr)
	} else {
		slog.Error("[CRITICAL] rollback failed", "orderID", p.OrderID, "amount", p.BalanceToDeduct, "error", rollbackErr)
	}
}
