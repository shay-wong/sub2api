//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateRefundRequestRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ORDER").
		SetOutTradeNo("sub2_refund_legacy_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	_, err = svc.validateRefundRequest(ctx, order.ID, user.ID)
	require.Error(t, err)
	require.Equal(t, "USER_REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy-admin@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-admin-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-admin-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(188).
		SetPayAmount(188).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ADMIN-ORDER").
		SetOutTradeNo("sub2_refund_legacy_admin_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-admin-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsPendingOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "prepare-rejects-pending")

	plan, result, err := (&PaymentService{entClient: client}).PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "INVALID_STATUS", infraerrors.Reason(err))
}

func TestPrepDeductBalanceRequiresForceWhenBalanceIsInsufficient(t *testing.T) {
	for _, tc := range []struct {
		name        string
		balance     float64
		force       bool
		wantDeduct  float64
		wantWarning bool
	}{
		{name: "insufficient balance", balance: 40, wantWarning: true},
		{name: "forced insufficient balance", balance: 40, force: true, wantDeduct: 40},
		{name: "equal balance", balance: 100, wantDeduct: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &RefundPlan{RefundAmount: 100}
			svc := &PaymentService{userRepo: &mockUserRepo{getByIDUser: &User{Balance: tc.balance}}}

			result := svc.prepDeduct(context.Background(), &dbent.PaymentOrder{
				UserID:    1,
				OrderType: payment.OrderTypeBalance,
			}, plan, tc.force)

			if tc.wantWarning {
				require.NotNil(t, result)
				require.False(t, result.Success)
				require.True(t, result.RequireForce)
				require.Equal(t, "user balance is insufficient for deduction, use force", result.Warning)
				require.Zero(t, plan.BalanceToDeduct)
				return
			}
			require.Nil(t, result)
			require.Equal(t, payment.DeductionTypeBalance, plan.DeductionType)
			require.Equal(t, tc.wantDeduct, plan.BalanceToDeduct)
		})
	}
}

func TestExecuteRefundImmediateSuccessDeductsAvailableBalanceOnce(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("refund-execute-clamp@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-execute-clamp").
		SetBalance(25).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-EXECUTE-CLAMP").
		SetOutTradeNo("refund_execute_clamp").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	deductionCalls := 0
	repo := &mockUserRepo{deductAvailableBalanceFn: func(txCtx context.Context, id int64, amount float64) (float64, error) {
		require.Equal(t, user.ID, id)
		require.Equal(t, 100.0, amount)
		tx := dbent.TxFromContext(txCtx)
		require.NotNil(t, tx)
		_, updateErr := tx.Client().User.UpdateOneID(id).AddBalance(-25).Save(txCtx)
		require.NoError(t, updateErr)
		deductionCalls++
		return 25, nil
	}}
	plan := &RefundPlan{
		OrderID: order.ID, Order: order, RefundAmount: 100, GatewayAmount: 100,
		Reason: "concurrent credit", Force: true, DeductionType: payment.DeductionTypeBalance,
	}

	result, err := (&PaymentService{entClient: client, userRepo: repo}).ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 25.0, plan.BalanceToDeduct)
	require.Equal(t, 25.0, result.BalanceDeducted)
	require.Equal(t, 1, deductionCalls)
	reloadedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Zero(t, reloadedUser.Balance)
	audit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Only(ctx)
	require.NoError(t, err)
	require.Contains(t, audit.Detail, `"balanceDeducted":25`)
}

func TestExecuteRefundRequiresForceWhenConcurrentSpendLeavesShortDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "execute-concurrent-spend-requires-force")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusCompleted).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	_, err = client.User.UpdateOneID(order.UserID).SetBalance(25).Save(ctx)
	require.NoError(t, err)

	provider := &refundCountingProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	repo := &mockUserRepo{
		getByIDUser: &User{ID: order.UserID, Balance: 100},
		deductAvailableBalanceFn: func(txCtx context.Context, id int64, amount float64) (float64, error) {
			tx := dbent.TxFromContext(txCtx)
			require.NotNil(t, tx)
			require.Equal(t, 100.0, amount)
			_, updateErr := tx.Client().User.UpdateOneID(id).AddBalance(-25).Save(txCtx)
			return 25, updateErr
		},
	}
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}, userRepo: repo}
	plan, result, err := svc.PrepareRefund(ctx, order.ID, 100, "concurrent spend", false, true)
	require.NoError(t, err)
	require.Nil(t, result)

	result, err = svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.True(t, result.RequireForce)
	require.Zero(t, provider.refundCalls)
	reloadedUser, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 25.0, reloadedUser.Balance)
	reloadedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloadedOrder.Status)
	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, pendingAudits)
}

func TestRefundFinalizePlanUsesOrderForceForLegacyAudit(t *testing.T) {
	plan := (&PaymentService{}).refundFinalizePlan(&dbent.PaymentOrder{
		ID:           42,
		RefundAmount: 10,
		ForceRefund:  true,
	}, refundPendingAuditDetail{HasPendingAudit: true})

	require.True(t, plan.Force)
}

func TestExecuteRefundRejectsPendingOrderBeforeProviderCall(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "execute-rejects-pending")
	provider := &refundCountingProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()

	result, err := (&PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}).ExecuteRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  order.RefundAmount,
		GatewayAmount: order.RefundAmount,
		Reason:        "retry pending refund",
	})
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	require.Zero(t, provider.refundCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
}

func TestRefundRetryBlockedWhileRollbackRequiresReconciliation(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "retry-blocked-by-rollback")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefundFailed).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_ROLLBACK_FAILED").
		SetOperator("admin").
		SetDetail(`{"balanceDeducted":100}`).
		Save(ctx)
	require.NoError(t, err)

	provider := &refundCountingProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 25, "changed retry", true, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_ROLLBACK_PENDING", infraerrors.Reason(err))

	result, err = svc.ExecuteRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  25,
		GatewayAmount: 25,
		Reason:        "changed retry",
	})
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_ROLLBACK_PENDING", infraerrors.Reason(err))
	require.Zero(t, provider.refundCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
}

func TestRefundRetryAllowedAfterRollbackFailureIsResolved(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "retry-after-rollback-resolution")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefundFailed).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_ROLLBACK_FAILED").
		SetOperator("admin").
		SetDetail(`{"balanceDeducted":100,"resolved":true}`).
		Save(ctx)
	require.NoError(t, err)

	provider := &refundCountingProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{
			getByIDUser:     &User{ID: order.UserID, Balance: 100},
			updateBalanceFn: func(context.Context, int64, float64) error { return nil },
		},
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 25, "changed retry", true, false)
	require.NoError(t, err)
	require.Nil(t, result)
	result, err = svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, 1, provider.refundCalls)
}

func TestRefundFailedWithoutPendingAuditRequiresReconciliation(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "failed-without-pending-audit")
	_, err := client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	order, err = client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusRefundFailed).
		SetRefundAmount(60).
		SetRefundReason("original refund reason").
		SetForceRefund(false).
		Save(ctx)
	require.NoError(t, err)

	provider := &refundCountingProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 25, "changed retry", true, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Equal(t, "REFUND_RECONCILIATION_REQUIRED", infraerrors.Reason(err))
	require.Zero(t, provider.refundCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
	require.Equal(t, 60.0, reloaded.RefundAmount)
	require.Equal(t, "original refund reason", psStringValue(reloaded.RefundReason))
	require.False(t, reloaded.ForceRefund)
}

func TestRefundAttemptClaimFencesPreparedStatusAndFailedGeneration(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "attempt-claim-fencing")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusCompleted).Save(ctx)
	require.NoError(t, err)

	provider := &refundQueryProviderTestDouble{
		refundCallResponse: &payment.RefundResponse{RefundID: "rf_failed", Status: payment.ProviderStatusFailed},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}

	firstPlan, result, err := svc.PrepareRefund(ctx, order.ID, 60, "first attempt", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	staleCompletedPlan, result, err := svc.PrepareRefund(ctx, order.ID, 25, "stale completed plan", true, false)
	require.NoError(t, err)
	require.Nil(t, result)

	result, err = svc.ExecuteRefund(ctx, firstPlan)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, 1, provider.refundCalls)

	result, err = svc.ExecuteRefund(ctx, staleCompletedPlan)
	require.Nil(t, result)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 1, provider.refundCalls)

	retryPlan, result, err := svc.PrepareRefund(ctx, order.ID, 25, "changed retry", true, true)
	require.NoError(t, err)
	require.Nil(t, result)
	staleRetryPlan, result, err := svc.PrepareRefund(ctx, order.ID, 10, "another changed retry", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, retryPlan.PreviousAttemptID, staleRetryPlan.PreviousAttemptID)

	result, err = svc.ExecuteRefund(ctx, retryPlan)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, 2, provider.refundCalls)

	result, err = svc.ExecuteRefund(ctx, staleRetryPlan)
	require.Nil(t, result)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 2, provider.refundCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
}

func TestGwRefundRejectsAlipayMerchantIdentitySnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-snapshot-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-snapshot-mismatch-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-mismatch-instance").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SNAPSHOT-MISMATCH-ORDER").
		SetOutTradeNo("sub2_refund_snapshot_mismatch_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-snapshot-mismatch").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeAlipay,
			"merchant_app_id":      "expected-alipay-app",
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	_, err = svc.prepareRefundProvider(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  order.Amount,
		GatewayAmount: order.Amount,
		Reason:        "snapshot mismatch",
	})
	require.ErrorContains(t, err, "alipay app_id mismatch")
}

func TestExecuteRefundValidatesProviderBeforeClaimAndDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "execute-validates-provider-first")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusCompleted).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": psStringValue(order.ProviderInstanceID),
			"provider_key":         payment.TypeStripe,
			"currency":             "EUR",
		}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)

	provider := &refundQueryProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	deductions := 0
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(context.Context, int64, float64) (float64, error) {
			deductions++
			return 100, nil
		}},
	}
	result, err := svc.ExecuteRefund(ctx, &RefundPlan{
		OrderID: order.ID, Order: order, RefundAmount: 100, GatewayAmount: 100,
		Reason: "metadata mismatch", Force: true, DeductionType: payment.DeductionTypeBalance, BalanceToDeduct: 100,
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "stripe currency mismatch")
	require.Zero(t, provider.refundCalls)
	require.Zero(t, deductions)
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, pendingAudits)
}

func TestExecuteRefundRejectsWxpayMerchantMismatchBeforeClaimAndDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "wxpay-metadata-mismatch")
	instanceID, err := strconv.ParseInt(psStringValue(order.ProviderInstanceID), 10, 64)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.UpdateOneID(instanceID).
		SetProviderKey(payment.TypeWxpay).
		SetSupportedTypes(payment.TypeWxpay).
		Save(ctx)
	require.NoError(t, err)
	order, err = client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusCompleted).
		SetPaymentType(payment.TypeWxpay).
		SetProviderKey(payment.TypeWxpay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": strconv.FormatInt(instanceID, 10),
			"provider_key":         payment.TypeWxpay,
			"merchant_app_id":      "expected-wx-app",
			"merchant_id":          "expected-mch",
			"currency":             "CNY",
		}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)

	provider := &wxpayRefundQueryProviderTestDouble{refundQueryProviderTestDouble: refundQueryProviderTestDouble{
		metadata: map[string]string{
			"appid":    "runtime-wx-app",
			"mp_appid": "runtime-wx-mp-app",
			"mchid":    "runtime-mch",
			"currency": "CNY",
		},
	}}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	deductions := 0
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(context.Context, int64, float64) (float64, error) {
			deductions++
			return 100, nil
		}},
	}
	result, err := svc.ExecuteRefund(ctx, &RefundPlan{
		OrderID: order.ID, Order: order, RefundAmount: 100, GatewayAmount: 100,
		Reason: "wxpay metadata mismatch", Force: true, DeductionType: payment.DeductionTypeBalance, BalanceToDeduct: 100,
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "wxpay appid mismatch")
	require.Zero(t, provider.refundCalls)
	require.Zero(t, deductions)
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, pendingAudits)
}

func TestCalculateGatewayRefundAmountUsesCurrencyPrecision(t *testing.T) {
	require.InDelta(t, 6.173, calculateGatewayRefundAmount(100, 12.345, 50, "KWD"), 1e-12)
	require.InDelta(t, 12.345, calculateGatewayRefundAmount(100, 12.345, 100, "KWD"), 1e-12)
	require.InDelta(t, 52, calculateGatewayRefundAmount(100, 103, 50, "JPY"), 1e-12)
}

func TestFormatGatewayRefundAmountUsesOrderCurrency(t *testing.T) {
	order := &dbent.PaymentOrder{
		ProviderSnapshot: map[string]any{
			"currency": "KWD",
		},
	}

	require.Equal(t, "12.345", formatGatewayRefundAmount(12.345, order))
}

func TestValidateRefundProviderResponseAcceptsPending(t *testing.T) {
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusPending}))
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusSuccess}))
	require.Error(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusFailed}))
	require.Error(t, validateRefundProviderResponse(nil))
}

func TestPendingProviderOutcomeMarksOrderPendingAndRollsBackDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-pending-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-PENDING-ORDER").
		SetOutTradeNo("sub2_refund_pending_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_refund_pending").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefunding).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	var rolledBack float64
	userRepo := &mockUserRepo{}
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		rolledBack += amount
		return nil
	}
	svc := &PaymentService{
		entClient: client,
		userRepo:  userRepo,
	}
	plan := &RefundPlan{
		OrderID:         order.ID,
		Order:           order,
		RefundAmount:    40,
		GatewayAmount:   40,
		Reason:          "gateway accepted but not final",
		Force:           true,
		DeductionType:   payment.DeductionTypeBalance,
		BalanceToDeduct: 40,
	}

	plan.DeductionApplied = true
	result, err := svc.settleRefundProviderOutcome(ctx, plan, &payment.RefundResponse{Status: payment.ProviderStatusPending}, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "pending confirmation")
	require.Equal(t, 40.0, rolledBack)
	require.Zero(t, plan.BalanceToDeduct)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	require.Equal(t, 40.0, reloaded.RefundAmount)
	require.NotNil(t, reloaded.RefundReason)
	require.Equal(t, "gateway accepted but not final", *reloaded.RefundReason)
	require.Nil(t, reloaded.RefundAt)

	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, pendingAudits)
	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, successAudits)
}

func TestFailedProviderOutcomeRestoresSubscriptionRevokedByDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "gateway-failure-restores-revoked-subscription")
	groupID := int64(77)
	days := 30
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusRefunding).
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(days).
		Save(ctx)
	require.NoError(t, err)

	subRepo := &refundUserSubscriptionRepoStub{active: &UserSubscription{
		ID:        9001,
		UserID:    order.UserID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().AddDate(0, 0, -1),
		ExpiresAt: time.Now().AddDate(0, 0, 5),
	}}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, nil, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	svc := &PaymentService{entClient: client, subscriptionSvc: subscriptionSvc}
	plan := &RefundPlan{
		OrderID:         order.ID,
		Order:           order,
		DeductBalance:   true,
		DeductionType:   payment.DeductionTypeSubscription,
		SubDaysToDeduct: days,
		SubscriptionID:  subRepo.active.ID,
	}
	originalExpiry := subRepo.active.ExpiresAt

	_, err = svc.applyRefundSubscriptionDeduction(ctx, plan, false)
	require.NoError(t, err)
	require.True(t, plan.SubscriptionRevoked)
	require.False(t, subRepo.deleted)
	require.Equal(t, SubscriptionStatusExpired, subRepo.active.Status)
	require.Equal(t, originalExpiry, plan.SubscriptionExpiresAtBeforeDeduction)
	require.Equal(t, subRepo.active.ExpiresAt, plan.SubscriptionExpiresAtAfterDeduction)

	plan.DeductionApplied = true
	result, err := svc.settleRefundProviderOutcome(ctx, plan, &payment.RefundResponse{Status: payment.ProviderStatusFailed}, errors.New("gateway rejected refund"))
	require.NoError(t, err)
	require.False(t, result.Success)
	require.False(t, subRepo.deleted)
	require.Equal(t, SubscriptionStatusActive, subRepo.active.Status)
	require.WithinDuration(t, originalExpiry, subRepo.active.ExpiresAt, time.Microsecond)
	require.Zero(t, subRepo.restoreCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
}

func TestRefundSubscriptionRevokeCacheFailureDoesNotMasqueradeAsMutationFailure(t *testing.T) {
	cache := &refundFailingSubscriptionCache{}
	subRepo := &refundUserSubscriptionRepoStub{active: &UserSubscription{
		ID:        9010,
		UserID:    10,
		GroupID:   20,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().AddDate(0, 0, -1),
		ExpiresAt: time.Now().AddDate(0, 0, 5),
	}}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, &BillingCacheService{cache: cache}, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	plan := &RefundPlan{
		OrderID:         100,
		Order:           &dbent.PaymentOrder{UserID: 10},
		DeductionType:   payment.DeductionTypeSubscription,
		SubDaysToDeduct: 30,
		SubscriptionID:  subRepo.active.ID,
	}

	_, err := (&PaymentService{subscriptionSvc: subscriptionSvc}).applyRefundSubscriptionDeduction(context.Background(), plan, false)
	require.NoError(t, err)
	require.True(t, plan.SubscriptionRevoked)
	require.False(t, subRepo.deleted)
	require.Equal(t, SubscriptionStatusExpired, subRepo.active.Status)
	require.Equal(t, 1, cache.invalidateCalls)
}

func TestRefundSubscriptionRestoreCacheFailureDoesNotMasqueradeAsRollbackFailure(t *testing.T) {
	deletedAt := time.Now().Add(-time.Minute)
	cache := &refundFailingSubscriptionCache{}
	subRepo := &refundUserSubscriptionRepoStub{
		active: &UserSubscription{
			ID:        9011,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().AddDate(0, 0, 5),
			DeletedAt: &deletedAt,
		},
		deleted: true,
	}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, &BillingCacheService{cache: cache}, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	plan := &RefundPlan{
		OrderID:             101,
		Order:               &dbent.PaymentOrder{UserID: 10},
		DeductionType:       payment.DeductionTypeSubscription,
		SubDaysToDeduct:     30,
		SubscriptionID:      subRepo.active.ID,
		SubscriptionRevoked: true,
	}

	_, err := (&PaymentService{subscriptionSvc: subscriptionSvc}).rollbackRefund(context.Background(), plan, false)
	require.NoError(t, err)
	require.False(t, subRepo.deleted)
	require.Equal(t, 1, subRepo.restoreCalls)
	require.Equal(t, 1, cache.invalidateCalls)
}

func TestPendingRefundFailureRestoresRevokedSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "pending-failure-restores-revoked-subscription")
	groupID := int64(78)
	days := 30
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusRefunding).
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(days).
		Save(ctx)
	require.NoError(t, err)

	subRepo := &refundUserSubscriptionRepoStub{active: &UserSubscription{
		ID:        9002,
		UserID:    order.UserID,
		GroupID:   groupID,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().AddDate(0, 0, -1),
		ExpiresAt: time.Now().AddDate(0, 0, 5),
	}}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, nil, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	svc := &PaymentService{entClient: client, subscriptionSvc: subscriptionSvc}
	plan := &RefundPlan{
		OrderID:         order.ID,
		Order:           order,
		RefundAmount:    order.RefundAmount,
		Reason:          "pending refund",
		DeductBalance:   true,
		DeductionType:   payment.DeductionTypeSubscription,
		SubDaysToDeduct: days,
		SubscriptionID:  subRepo.active.ID,
	}

	_, err = svc.applyRefundSubscriptionDeduction(ctx, plan, false)
	require.NoError(t, err)
	require.True(t, plan.SubscriptionRevoked)
	subRepo.extendFailures = 1

	plan.DeductionApplied = true
	result, err := svc.settleRefundProviderOutcome(ctx, plan, &payment.RefundResponse{RefundID: "rf_revoked", Status: payment.ProviderStatusPending}, nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "rollback failed")

	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, pendingDetail.SubscriptionRevoked)
	require.False(t, pendingDetail.DeductionRollbackOK)
	require.False(t, subRepo.deleted)
	require.Equal(t, SubscriptionStatusExpired, subRepo.active.Status)

	result, err = svc.finalizeRefundFailed(ctx, order, pendingDetail, errors.New("provider confirmed failure"))
	require.NoError(t, err)
	require.False(t, result.Success)
	require.False(t, subRepo.deleted)
	require.Equal(t, SubscriptionStatusActive, subRepo.active.Status)
	require.Zero(t, subRepo.restoreCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
}

func TestRollbackRefundRestoresRevokedSubscriptionFromLegacyPendingAudit(t *testing.T) {
	deletedAt := time.Now().Add(-time.Minute)
	subRepo := &refundUserSubscriptionRepoStub{
		active: &UserSubscription{
			ID:        9003,
			UserID:    10,
			GroupID:   20,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().AddDate(0, 0, 5),
			DeletedAt: &deletedAt,
		},
		deleted: true,
	}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, nil, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	plan := &RefundPlan{
		Order:           &dbent.PaymentOrder{UserID: 10},
		DeductionType:   payment.DeductionTypeSubscription,
		SubDaysToDeduct: 30,
		SubscriptionID:  subRepo.active.ID,
	}

	_, err := (&PaymentService{subscriptionSvc: subscriptionSvc}).rollbackRefund(context.Background(), plan, false)
	require.NoError(t, err)
	require.True(t, plan.SubscriptionRevoked)
	require.False(t, subRepo.deleted)
	require.Equal(t, 1, subRepo.restoreCalls)
}

func TestRollbackRefundPreservesSubscriptionLookupFailure(t *testing.T) {
	dbErr := errors.New("injected subscription lookup failure")
	subRepo := &refundUserSubscriptionRepoStub{getByIDErr: dbErr}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, nil, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	plan := &RefundPlan{
		Order:           &dbent.PaymentOrder{UserID: 10},
		DeductionType:   payment.DeductionTypeSubscription,
		SubDaysToDeduct: 30,
		SubscriptionID:  9004,
	}

	_, err := (&PaymentService{subscriptionSvc: subscriptionSvc}).rollbackRefund(context.Background(), plan, false)
	require.ErrorIs(t, err, dbErr)
	require.Zero(t, subRepo.getByIDIncludeDeletedCalls)
}

func TestRollbackRefundPreservesLegacySubscriptionLookupFailure(t *testing.T) {
	dbErr := errors.New("injected legacy subscription lookup failure")
	subRepo := &refundUserSubscriptionRepoStub{
		deleted:                  true,
		getByIDIncludeDeletedErr: dbErr,
	}
	subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, nil, nil, nil)
	t.Cleanup(subscriptionSvc.Stop)
	plan := &RefundPlan{
		Order:           &dbent.PaymentOrder{UserID: 10},
		DeductionType:   payment.DeductionTypeSubscription,
		SubDaysToDeduct: 30,
		SubscriptionID:  9005,
	}

	_, err := (&PaymentService{subscriptionSvc: subscriptionSvc}).rollbackRefund(context.Background(), plan, false)
	require.ErrorIs(t, err, dbErr)
	require.ErrorContains(t, err, "lookup legacy revoked subscription")
	require.Equal(t, 1, subRepo.getByIDIncludeDeletedCalls)
}

func TestSuccessfulProviderOutcomeFinalizesRefund(t *testing.T) {
	for _, status := range []string{payment.ProviderStatusSuccess, payment.ProviderStatusRefunded} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			user, err := client.User.Create().
				SetEmail("refund-success-" + status + "@example.com").
				SetPasswordHash("hash").
				SetUsername("refund-success-" + status).
				Save(ctx)
			require.NoError(t, err)

			order, err := client.PaymentOrder.Create().
				SetUserID(user.ID).
				SetUserEmail(user.Email).
				SetUserName(user.Username).
				SetAmount(100).
				SetPayAmount(100).
				SetFeeRate(0).
				SetRechargeCode("REFUND-SUCCESS-" + status).
				SetOutTradeNo("sub2_refund_success_" + status).
				SetPaymentType(payment.TypeStripe).
				SetPaymentTradeNo("pi_refund_success_" + status).
				SetOrderType(payment.OrderTypeBalance).
				SetStatus(OrderStatusRefunding).
				SetExpiresAt(time.Now().Add(time.Hour)).
				SetPaidAt(time.Now()).
				SetClientIP("127.0.0.1").
				SetSrcHost("api.example.com").
				Save(ctx)
			require.NoError(t, err)

			svc := &PaymentService{entClient: client}
			plan := &RefundPlan{
				OrderID:           order.ID,
				Order:             order,
				AttemptID:         "attempt-" + status,
				ProviderRequestID: "provider-request-" + status,
				RefundAmount:      100,
				GatewayAmount:     100,
				Reason:            "final success",
				DeductBalance:     true,
				DeductionType:     payment.DeductionTypeBalance,
				BalanceToDeduct:   100,
			}

			plan.DeductionApplied = true
			require.NoError(t, svc.upsertRefundAudit(ctx, client, order.ID, "REFUND_PENDING", refundAttemptAuditDetail(plan, "", false, "dispatching")))
			result, err := svc.settleRefundProviderOutcome(ctx, plan, &payment.RefundResponse{RefundID: "refund-" + status, Status: status}, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Success)
			require.Equal(t, 100.0, result.BalanceDeducted)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusRefunded, reloaded.Status)
			require.NotNil(t, reloaded.RefundAt)

			successAudit, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
				Only(ctx)
			require.NoError(t, err)
			var terminalDetail refundPendingAuditDetail
			require.NoError(t, json.Unmarshal([]byte(successAudit.Detail), &terminalDetail))
			require.Equal(t, plan.AttemptID, terminalDetail.AttemptID)
			require.Equal(t, plan.ProviderRequestID, terminalDetail.ProviderRequestID)
			require.Equal(t, "refund-"+status, terminalDetail.RefundID)
			require.True(t, terminalDetail.DeductionApplied)
			require.Equal(t, 100.0, terminalDetail.BalanceToDeduct)
			pendingAudits, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
				Count(ctx)
			require.NoError(t, err)
			require.Zero(t, pendingAudits)
		})
	}
}

func TestQueryAndFinalizeRefundFinalizesProviderStatuses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		wantStatus string
		wantDeduct float64
		available  float64
	}{
		{name: "success", status: payment.ProviderStatusSuccess, wantStatus: OrderStatusRefunded, wantDeduct: 100, available: 100},
		{name: "failed", status: payment.ProviderStatusFailed, wantStatus: OrderStatusRefundFailed},
		{name: "pending", status: payment.ProviderStatusPending, wantStatus: OrderStatusRefundPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-"+tc.name)

			var deducted float64
			svc := &PaymentService{
				entClient:    client,
				loadBalancer: &captureLoadBalancer{},
				userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
					deducted += tc.available
					return tc.available, nil
				}},
			}
			restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
				refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: tc.status},
			})
			defer restore()

			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tc.status == payment.ProviderStatusSuccess, result.Success)
			require.Equal(t, tc.wantDeduct, deducted)
			if tc.status == payment.ProviderStatusSuccess {
				require.Equal(t, tc.wantDeduct, result.BalanceDeducted)
				audit, err := client.PaymentAuditLog.Query().
					Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
					Only(ctx)
				require.NoError(t, err)
				require.Contains(t, audit.Detail, fmt.Sprintf(`"balanceDeducted":%v`, tc.wantDeduct))
			}

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, reloaded.Status)
		})
	}
}

func TestQueryAndFinalizeRefundRequiresFullBalanceForNonForceRefund(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-short-deduction")

	var requested float64
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
			requested = amount
			return 35, nil
		}},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "full balance deduction")
	require.Equal(t, 100.0, requested)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, successAudits)
}

func TestQueryAndFinalizeRefundForceUsesLatestAvailableBalance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-force-latest-balance")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetForceRefund(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail(`{"refundID":"rf_old","refundAmount":100,"force":true,"deductBalance":true,"deductionType":"balance","balanceToDeduct":0,"deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)

	var requested float64
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, _ int64, amount float64) (float64, error) {
			require.NotNil(t, dbent.TxFromContext(ctx))
			requested = amount
			return 60, nil
		}},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_latest", Status: payment.ProviderStatusSuccess},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 100.0, requested)
	require.Equal(t, 60.0, result.BalanceDeducted)

	successAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Only(ctx)
	require.NoError(t, err)
	var detail refundPendingAuditDetail
	require.NoError(t, json.Unmarshal([]byte(successAudit.Detail), &detail))
	require.Equal(t, "rf_latest", detail.RefundID)
	require.Equal(t, 60.0, detail.BalanceToDeduct)
}

func TestQueryAndFinalizeRefundFailedPersistsObservedRefundID(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-failed-refund-id")
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_failed_query", Status: payment.ProviderStatusFailed},
	})
	defer restore()

	result, err := (&PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}).QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, result.Success)

	detail, err := (&PaymentService{entClient: client}).latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, "rf_failed_query", detail.RefundID)
	require.Equal(t, "provider_confirmed", detail.Phase)
	require.True(t, detail.DeductionRollbackOK)
}

func TestQueryAndFinalizeRefundPreservesNoDeductionIntent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-no-deduction")

	_, err := client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductBalance":false,"deductionType":"none","deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)

	var deducted float64
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			deducted += amount
			return amount, nil
		}},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Zero(t, deducted)
	require.Zero(t, result.BalanceDeducted)
}

func TestQueryAndFinalizeRefundFinalizesLegacyPendingAuditWithDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-legacy-pending")

	_, err := client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)

	var deducted float64
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			deducted += amount
			return amount, nil
		}},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 100.0, deducted)
	require.Equal(t, 100.0, result.BalanceDeducted)
}

func TestQueryAndFinalizeRefundRequiresManualReconciliationForLegacyAlipayAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-legacy-alipay")
	provider := &alipayRefundQueryProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()

	result, err := (&PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}).QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Equal(t, "REFUND_RECONCILIATION_REQUIRED", infraerrors.Reason(err))
	require.Zero(t, provider.queryCalls)
	require.Zero(t, provider.refundCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
}

func TestQueryAndFinalizeRefundPreservesLegacyWxpayRefundID(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-legacy-gateway-amount")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetAmount(100).
		SetPayAmount(80).
		SetRefundAmount(50).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail(`{"deductBalance":false,"deductionType":"none","deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)

	provider := &wxpayRefundQueryProviderTestDouble{refundQueryProviderTestDouble: refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	}}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()

	result, err := (&PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}).QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "40.00", provider.lastQueryRequest.Amount)
	require.Empty(t, provider.lastQueryRequest.RefundID)
	successAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Only(ctx)
	require.NoError(t, err)
	var terminalDetail refundPendingAuditDetail
	require.NoError(t, json.Unmarshal([]byte(successAudit.Detail), &terminalDetail))
	require.Equal(t, "rf_test", terminalDetail.RefundID)
	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, pendingAudits)
}

func TestQueryAndFinalizeRefundFailsClosedOnPendingAuditQueryError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-audit-query-error")
	auditErr := errors.New("injected pending audit query failure")
	client.PaymentAuditLog.Intercept(ent.InterceptFunc(func(ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
			return nil, auditErr
		})
	}))

	provider := &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	deductions := 0
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(context.Context, int64, float64) (float64, error) {
			deductions++
			return 100, nil
		}},
	}

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.ErrorIs(t, err, auditErr)
	require.Zero(t, provider.queryCalls)
	require.Zero(t, deductions)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
}

func TestQueryAndFinalizeRefundFailsClosedOnCorruptPendingAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-corrupt-audit")
	_, err := client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail("null").
		Save(ctx)
	require.NoError(t, err)

	provider := &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	deductions := 0
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(context.Context, int64, float64) (float64, error) {
			deductions++
			return 100, nil
		}},
	}

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.ErrorContains(t, err, "expected object")
	require.Zero(t, provider.queryCalls)
	require.Zero(t, deductions)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
}

func TestRefundRetryPreservesParametersAndReplacesPendingAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "retry-replaces-pending-audit")
	ensurePaymentAuditIdempotencyIndexes(t, ctx, client)
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetRefundAmount(60).
		SetRefundReason("original refund reason").
		SetForceRefund(false).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail(`{"providerRequestID":"provider-original","refundID":"rf_original","refundAmount":60,"reason":"original refund reason","force":false,"deductBalance":true,"deductionType":"balance","balanceToDeduct":60,"deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)

	provider := &refundQueryProviderTestDouble{
		refundResponse:     &payment.RefundResponse{RefundID: "rf_original", Status: payment.ProviderStatusFailed},
		refundCallResponse: &payment.RefundResponse{RefundID: "rf_retry", Status: payment.ProviderStatusPending},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	var deducted, rolledBack float64
	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: order.UserID, Balance: 100},
		deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
			deducted += amount
			return amount, nil
		},
		updateBalanceFn: func(_ context.Context, _ int64, amount float64) error {
			rolledBack += amount
			return nil
		},
	}
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}, userRepo: userRepo}

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, 1, provider.queryCalls)
	require.Equal(t, "provider-original", provider.lastQueryRequest.ProviderRequestID)
	require.Equal(t, "rf_original", provider.lastQueryRequest.RefundID)
	require.Equal(t, "60.00", provider.lastQueryRequest.Amount)

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 25, "changed retry reason", true, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, 60.0, plan.RefundAmount)
	require.Equal(t, "original refund reason", plan.Reason)
	require.False(t, plan.Force)
	require.True(t, plan.DeductBalance)
	require.Equal(t, payment.DeductionTypeBalance, plan.DeductionType)
	require.Equal(t, 60.0, plan.BalanceToDeduct)

	result, err = svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, 1, provider.refundCalls)
	require.NotEmpty(t, provider.lastRefundRequest.ProviderRequestID)
	require.NotEqual(t, "provider-original", provider.lastRefundRequest.ProviderRequestID)
	require.Equal(t, "60.00", provider.lastRefundRequest.Amount)
	require.Equal(t, "original refund reason", provider.lastRefundRequest.Reason)
	require.Equal(t, 60.0, deducted)
	require.Equal(t, 60.0, rolledBack)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	require.Equal(t, 60.0, reloaded.RefundAmount)
	require.Equal(t, "original refund reason", psStringValue(reloaded.RefundReason))
	require.False(t, reloaded.ForceRefund)
	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, pendingAudits, 1)
	var pendingDetail struct {
		ProviderRequestID   string  `json:"providerRequestID"`
		RefundID            string  `json:"refundID"`
		RefundAmount        float64 `json:"refundAmount"`
		Reason              string  `json:"reason"`
		Force               bool    `json:"force"`
		DeductBalance       bool    `json:"deductBalance"`
		BalanceToDeduct     float64 `json:"balanceToDeduct"`
		DeductionRollbackOK bool    `json:"deductionRollbackOK"`
	}
	require.NoError(t, json.Unmarshal([]byte(pendingAudits[0].Detail), &pendingDetail))
	require.Equal(t, provider.lastRefundRequest.ProviderRequestID, pendingDetail.ProviderRequestID)
	require.Equal(t, "rf_retry", pendingDetail.RefundID)
	require.Equal(t, 60.0, pendingDetail.RefundAmount)
	require.Equal(t, "original refund reason", pendingDetail.Reason)
	require.False(t, pendingDetail.Force)
	require.True(t, pendingDetail.DeductBalance)
	require.Equal(t, 60.0, pendingDetail.BalanceToDeduct)
	require.True(t, pendingDetail.DeductionRollbackOK)
}

func TestQueryAndFinalizeRefundRetriesRollbackBeforeMarkingFailed(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-compensates-failed")

	_, err := client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductBalance":true,"deductionType":"balance","balanceToDeduct":100,"deductionRollbackOK":false}`).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_ROLLBACK_FAILED").
		SetOperator("admin").
		SetDetail(`{"balanceDeducted":100,"gatewayError":"initial rollback failure","rollbackError":"db unavailable","resolved":false}`).
		Save(ctx)
	require.NoError(t, err)

	var compensated float64
	provider := &refundQueryProviderTestDouble{
		refundResponse:     &payment.RefundResponse{RefundID: "rf_failed_query", Status: payment.ProviderStatusFailed},
		refundCallResponse: &payment.RefundResponse{RefundID: "rf_retry", Status: payment.ProviderStatusPending},
	}
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{
			getByIDUser: &User{ID: order.UserID, Balance: 100},
			updateBalanceFn: func(ctx context.Context, id int64, amount float64) error {
				require.NotNil(t, dbent.TxFromContext(ctx))
				compensated += amount
				return nil
			},
		},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, 100.0, compensated)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
	pendingAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Only(ctx)
	require.NoError(t, err)
	var compensatedDetail refundPendingAuditDetail
	require.NoError(t, json.Unmarshal([]byte(pendingAudit.Detail), &compensatedDetail))
	require.Equal(t, "rf_failed_query", compensatedDetail.RefundID)
	require.True(t, compensatedDetail.DeductionRollbackOK)
	require.Equal(t, 100.0, compensatedDetail.BalanceRolledBack)
	require.Zero(t, compensatedDetail.SubDaysRolledBack)
	rollbackAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_ROLLBACK_FAILED")).
		Only(ctx)
	require.NoError(t, err)
	var rollbackDetail struct {
		Resolved   bool   `json:"resolved"`
		ResolvedAt string `json:"resolvedAt"`
	}
	require.NoError(t, json.Unmarshal([]byte(rollbackAudit.Detail), &rollbackDetail))
	require.True(t, rollbackDetail.Resolved)
	require.NotEmpty(t, rollbackDetail.ResolvedAt)
	require.NoError(t, svc.ensureRefundRetryAllowed(ctx, order.ID))

	plan, retryResult, err := svc.PrepareRefund(ctx, order.ID, 25, "changed retry", true, false)
	require.NoError(t, err)
	require.Nil(t, retryResult)
	retryResult, err = svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, retryResult.Success)
	require.Equal(t, 1, provider.refundCalls)
}

func TestRefundingAttemptRecoversFromProviderOutcomePersistenceFailure(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "refunding-outcome-recovery")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusCompleted).Save(ctx)
	require.NoError(t, err)

	provider := &refundQueryProviderTestDouble{
		refundResponse:     &payment.RefundResponse{RefundID: "rf_recovered", Status: payment.ProviderStatusPending},
		refundCallResponse: &payment.RefundResponse{RefundID: "rf_recovered", Status: payment.ProviderStatusPending},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()

	auditWrites := 0
	auditErr := errors.New("injected provider outcome audit failure")
	client.PaymentAuditLog.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if mutation.Op().Is(ent.OpCreate) {
				auditWrites++
				if auditWrites == 2 {
					return nil, auditErr
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})

	var rolledBack float64
	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{
			getByIDUser: &User{ID: order.UserID, Balance: 100},
			updateBalanceFn: func(_ context.Context, _ int64, amount float64) error {
				rolledBack += amount
				return nil
			},
		},
	}
	plan, result, err := svc.PrepareRefund(ctx, order.ID, 60, "recover interrupted outcome", true, true)
	require.NoError(t, err)
	require.Nil(t, result)
	result, err = svc.ExecuteRefund(ctx, plan)
	require.Nil(t, result)
	require.ErrorIs(t, err, auditErr)
	require.Equal(t, 1, provider.refundCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunding, reloaded.Status)
	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, "dispatching", pendingDetail.Phase)
	require.NotEmpty(t, pendingDetail.AttemptID)
	require.NotEmpty(t, pendingDetail.ProviderRequestID)
	require.Equal(t, plan.ProviderRequestID, pendingDetail.ProviderRequestID)
	require.True(t, pendingDetail.DeductionApplied)
	require.Equal(t, 60.0, pendingDetail.BalanceToDeduct)

	result, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Equal(t, "REFUND_IN_PROGRESS", infraerrors.Reason(err))
	require.Zero(t, provider.queryCalls)

	pendingAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Only(ctx)
	require.NoError(t, err)
	var pendingRaw map[string]any
	require.NoError(t, json.Unmarshal([]byte(pendingAudit.Detail), &pendingRaw))
	pendingRaw["attemptStartedAt"] = time.Now().Add(-refundRecoveryDelay - time.Second).UTC()
	backdatedDetail, err := json.Marshal(pendingRaw)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.UpdateOneID(pendingAudit.ID).SetDetail(string(backdatedDetail)).Save(ctx)
	require.NoError(t, err)

	result, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, 1, provider.queryCalls)
	require.Equal(t, 1, provider.refundCalls)
	require.Equal(t, plan.ProviderRequestID, provider.lastQueryRequest.ProviderRequestID)
	require.Equal(t, 60.0, rolledBack)

	reloaded, err = client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	pendingDetail, err = svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, "provider_confirmed", pendingDetail.Phase)
	require.NotEqual(t, plan.AttemptID, pendingDetail.AttemptID)
	require.Equal(t, plan.ProviderRequestID, pendingDetail.ProviderRequestID)
	require.True(t, pendingDetail.DeductionRollbackOK)
}

func TestDelayedRefundOutcomeCannotClaimNewAttempt(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "delayed-outcome-attempt-cas")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunding).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail(`{"attemptID":"attempt-b","attemptStartedAt":"2026-08-03T00:00:00Z","phase":"dispatching","previousStatus":"REFUND_FAILED","refundAmount":100,"reason":"new attempt","deductBalance":false,"deductionType":"none","deductionApplied":true,"deductionRollbackOK":false}`).
		Save(ctx)
	require.NoError(t, err)

	result, err := (&PaymentService{entClient: client}).settleRefundProviderOutcome(ctx, &RefundPlan{
		OrderID: order.ID, Order: order, AttemptID: "attempt-a", AttemptStartedAt: time.Now().Add(-time.Hour),
		PreviousStatus: OrderStatusCompleted, RefundAmount: 100, GatewayAmount: 100, Reason: "old attempt", DeductionApplied: true,
	}, &payment.RefundResponse{RefundID: "rf_old", Status: payment.ProviderStatusPending}, nil)
	require.Nil(t, result)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunding, reloaded.Status)
	detail, err := (&PaymentService{entClient: client}).latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, "attempt-b", detail.AttemptID)
	require.Equal(t, "dispatching", detail.Phase)
}

func TestRefundingAttemptRequiresManualReconciliationForUnsafeProvider(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "refunding-unsafe-provider")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunding).Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail(`{"phase":"dispatching","previousStatus":"COMPLETED","refundAmount":100,"reason":"unsafe recovery","deductBalance":false,"deductionType":"none","deductionApplied":false,"deductionRollbackOK":false}`).
		Save(ctx)
	require.NoError(t, err)

	provider := &unsafeRefundProviderTestDouble{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	result, err := (&PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}).QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Equal(t, "REFUND_RECONCILIATION_REQUIRED", infraerrors.Reason(err))
	require.Zero(t, provider.refundCalls)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunding, reloaded.Status)
}

func TestRefundingAttemptDoesNotReplayUnsafeQueryProvider(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "refunding-unsafe-query-provider")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunding).Save(ctx)
	require.NoError(t, err)
	backdated := time.Now().Add(-refundRecoveryDelay - time.Second).UTC().Format(time.RFC3339Nano)
	_, err = client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail(fmt.Sprintf(`{"attemptID":"unsafe-query-attempt","providerRequestID":"unsafe-query-request","attemptStartedAt":%q,"phase":"dispatching","previousStatus":"COMPLETED","refundAmount":100,"reason":"unsafe recovery","deductBalance":false,"deductionType":"none","deductionApplied":false,"deductionRollbackOK":false}`, backdated)).
		Save(ctx)
	require.NoError(t, err)

	provider := &unsafeRefundQueryProviderTestDouble{refundQueryProviderTestDouble: refundQueryProviderTestDouble{
		queryErr:           errors.New("query unavailable"),
		refundCallResponse: &payment.RefundResponse{RefundID: "unsafe", Status: payment.ProviderStatusPending},
	}}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	result, err := (&PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}).QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Equal(t, "REFUND_RECONCILIATION_REQUIRED", infraerrors.Reason(err))
	require.Equal(t, 1, provider.queryCalls)
	require.Zero(t, provider.refundCalls)
}

func TestRefundingAttemptUsesFreshContextForReplay(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "refunding-fresh-replay-context")
	order, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunding).Save(ctx)
	require.NoError(t, err)
	backdated := time.Now().Add(-refundRecoveryDelay - time.Second).UTC().Format(time.RFC3339Nano)
	_, err = client.PaymentAuditLog.Update().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		SetDetail(fmt.Sprintf(`{"attemptID":"fresh-context-attempt","providerRequestID":"fresh-context-request","attemptStartedAt":%q,"phase":"dispatching","previousStatus":"COMPLETED","refundAmount":100,"reason":"safe recovery","deductBalance":false,"deductionType":"none","deductionApplied":false,"deductionRollbackOK":false}`, backdated)).
		Save(ctx)
	require.NoError(t, err)

	provider := &refundQueryProviderTestDouble{
		queryErr:           errors.New("query unavailable"),
		refundCallResponse: &payment.RefundResponse{RefundID: "rf_replayed", Status: payment.ProviderStatusPending},
	}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	result, err := (&PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}).QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Equal(t, 1, provider.queryCalls)
	require.Equal(t, 1, provider.refundCalls)
	require.NotEqual(t, provider.queryContext, provider.refundContext)
	require.ErrorIs(t, provider.queryContext.Err(), context.Canceled)
	require.NoError(t, provider.refundContextErr)
	require.Equal(t, provider.lastQueryRequest.ProviderRequestID, provider.lastRefundRequest.ProviderRequestID)
}

func TestQueryAndFinalizeRefundRetriesLegacySubscriptionRollbackBeforeMarkingFailed(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-legacy-sub-compensates")
	groupID := int64(77)
	days := 30
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(days).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","subDaysDeducted":30,"deductionRollbackOK":false}`).
		Save(ctx)
	require.NoError(t, err)

	subRepo := &refundUserSubscriptionRepoStub{
		active: &UserSubscription{
			ID:        9001,
			UserID:    order.UserID,
			GroupID:   groupID,
			Status:    SubscriptionStatusActive,
			StartsAt:  time.Now().AddDate(0, 0, -1),
			ExpiresAt: time.Now().AddDate(0, 0, 29),
		},
	}
	svc := &PaymentService{
		entClient:       client,
		loadBalancer:    &captureLoadBalancer{},
		subscriptionSvc: &SubscriptionService{userSubRepo: subRepo},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusFailed},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, int64(9001), subRepo.extendID)
	require.Equal(t, 30, subRepo.extendDays)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundFailed, reloaded.Status)
}

func TestQueryAndFinalizeRefundKeepsPendingWhenFailedRollbackStillFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-compensation-fails")

	_, err := client.PaymentAuditLog.Delete().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Exec(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"attemptID":"rollback-attempt","providerRequestID":"rollback-request","refundID":"rf_old","deductBalance":true,"deductionType":"balance","balanceToDeduct":100,"deductionRollbackOK":false}`).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: &captureLoadBalancer{},
		userRepo:     &mockUserRepo{updateBalanceErr: errors.New("db unavailable")},
	}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_failed_query", Status: payment.ProviderStatusFailed},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refund failed but deduction rollback is still pending")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, "rf_failed_query", pendingDetail.RefundID)
	require.Equal(t, "rollback-attempt", pendingDetail.AttemptID)
	require.Equal(t, "rollback-request", pendingDetail.ProviderRequestID)
	require.Equal(t, "provider_confirmed", pendingDetail.Phase)
	require.False(t, pendingDetail.DeductionRollbackOK)
	rollbackAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_ROLLBACK_FAILED")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, rollbackAudits)
}

func TestQueryAndFinalizeRefundRejectsProviderSnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-snapshot-mismatch")

	_, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": psStringValue(order.ProviderInstanceID),
			"provider_key":         payment.TypeStripe,
			"currency":             "EUR",
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
		refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.ErrorContains(t, err, "stripe currency mismatch")
}

func TestFinalizePendingRefundSuccessRejectsStaleCallerBeforeSecondDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "finalize-stale")

	deductions := 0
	svc := &PaymentService{
		entClient: client,
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			require.NotNil(t, dbent.TxFromContext(ctx))
			deductions++
			return amount, nil
		}},
	}

	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	first, err := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order, pendingDetail))
	require.NoError(t, err)
	require.True(t, first.Success)

	second, err := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order, pendingDetail))
	require.Nil(t, second)
	require.Error(t, err)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 1, deductions)

	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, successAudits)
}

func TestPendingRefundSubscriptionMutationInvalidatesL1AfterCommit(t *testing.T) {
	for _, tc := range []struct {
		name         string
		rollbackOK   bool
		expiresDelta int
		wantStatus   string
		wantSuccess  bool
	}{
		{name: "final success deducts", rollbackOK: true, expiresDelta: -10, wantStatus: OrderStatusRefunded, wantSuccess: true},
		{name: "final failure compensates", rollbackOK: false, expiresDelta: 10, wantStatus: OrderStatusRefundFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, "subscription-cache-"+tc.name)
			groupID := int64(79)
			days := 10
			order, err := client.PaymentOrder.UpdateOneID(order.ID).
				SetOrderType(payment.OrderTypeSubscription).
				SetSubscriptionGroupID(groupID).
				SetSubscriptionDays(days).
				Save(ctx)
			require.NoError(t, err)

			_, err = client.PaymentAuditLog.Delete().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
				Exec(ctx)
			require.NoError(t, err)
			_, err = client.PaymentAuditLog.Create().
				SetOrderID(strconv.FormatInt(order.ID, 10)).
				SetAction("REFUND_PENDING").
				SetOperator("admin").
				SetDetail(fmt.Sprintf(`{"refundID":"rf_cache","deductBalance":true,"deductionType":"subscription","subDaysToDeduct":%d,"subscriptionID":9004,"deductionRollbackOK":%t}`, days, tc.rollbackOK)).
				Save(ctx)
			require.NoError(t, err)

			initialExpiresAt := time.Now().AddDate(0, 0, 40)
			subRepo := &refundUserSubscriptionRepoStub{active: &UserSubscription{
				ID:        9004,
				UserID:    order.UserID,
				GroupID:   groupID,
				Status:    SubscriptionStatusActive,
				StartsAt:  time.Now().AddDate(0, 0, -1),
				ExpiresAt: initialExpiresAt,
			}}
			subscriptionSvc := NewSubscriptionService(groupRepoNoop{}, subRepo, nil, nil, &config.Config{
				SubscriptionCache: config.SubscriptionCacheConfig{L1Size: 100, L1TTLSeconds: 60},
			})
			t.Cleanup(subscriptionSvc.Stop)

			cacheKey := subCacheKey(order.UserID, groupID)
			cachedSubscription := *subRepo.active
			require.True(t, subscriptionSvc.subCacheL1.Set(cacheKey, &cachedSubscription, 1))
			subscriptionSvc.subCacheL1.Wait()
			_, cachedBeforeTx := subscriptionSvc.subCacheL1.Get(cacheKey)
			require.True(t, cachedBeforeTx)
			require.Zero(t, subRepo.getActiveCalls)

			cacheObservedInsideTx := false
			subRepo.onExtendExpiry = func(updateCtx context.Context) {
				require.NotNil(t, dbent.TxFromContext(updateCtx))
				cached, ok := subscriptionSvc.subCacheL1.Get(cacheKey)
				require.True(t, ok, "cache must remain populated until the outer transaction commits")
				require.Equal(t, initialExpiresAt, cached.(*UserSubscription).ExpiresAt)
				cacheObservedInsideTx = true
			}

			svc := &PaymentService{entClient: client, subscriptionSvc: subscriptionSvc}
			pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
			require.NoError(t, err)
			var result *RefundResult
			if tc.rollbackOK {
				result, err = svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order, pendingDetail))
			} else {
				result, err = svc.finalizeRefundFailed(ctx, order, pendingDetail, errors.New("provider confirmed failure"))
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantSuccess, result.Success)
			require.True(t, cacheObservedInsideTx)
			_, cachedAfterCommit := subscriptionSvc.subCacheL1.Get(cacheKey)
			require.False(t, cachedAfterCommit, "committed mutation must synchronously invalidate L1")

			refreshed, err := subscriptionSvc.GetActiveSubscription(ctx, order.UserID, groupID)
			require.NoError(t, err)
			require.Equal(t, 1, subRepo.getActiveCalls)
			require.Equal(t, initialExpiresAt.AddDate(0, 0, tc.expiresDelta), refreshed.ExpiresAt)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, reloaded.Status)
		})
	}
}

func TestFinalizeRefundFailedRejectsStaleCallerAfterSuccess(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "finalize-stale-failure")

	deductions := 0
	svc := &PaymentService{
		entClient: client,
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			require.NotNil(t, dbent.TxFromContext(ctx))
			deductions++
			return amount, nil
		}},
	}
	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)

	result, err := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order, pendingDetail))
	require.NoError(t, err)
	require.True(t, result.Success)

	result, err = svc.finalizeRefundFailed(ctx, order, pendingDetail, errors.New("stale provider failure"))
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "CONFLICT", infraerrors.Reason(err))
	require.Equal(t, 1, deductions)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, reloaded.Status)
	failedAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_FAILED")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, failedAudits)
}

func TestFinalizePendingRefundSuccessRollsBackPostDeductionFailure(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "finalize-rollback")
	_, err := client.User.UpdateOneID(order.UserID).SetBalance(100).Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(ctx context.Context, id int64, amount float64) (float64, error) {
			tx := dbent.TxFromContext(ctx)
			require.NotNil(t, tx)
			if _, updateErr := tx.Client().User.UpdateOneID(id).AddBalance(-amount).Save(ctx); updateErr != nil {
				return 0, updateErr
			}
			return 0, errors.New("injected failure after deduction")
		}},
	}

	pendingDetail, err := svc.latestRefundPendingDetail(ctx, order.ID)
	require.NoError(t, err)
	result, err := svc.finalizePendingRefundSuccess(ctx, svc.refundFinalizePlan(order, pendingDetail))
	require.Nil(t, result)
	require.ErrorContains(t, err, "injected failure after deduction")

	user, err := client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	require.Equal(t, 100.0, user.Balance)
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, successAudits)
}

func TestQueryAndFinalizeRefundUnsupportedProviderReturnsClearError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-unsupported")
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	restore := replacePaymentProviderFactoryForTest(t, refundProviderTestDouble{})
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_QUERY_UNSUPPORTED", infraerrors.Reason(err))
}

func createPendingRefundOrderForTest(t *testing.T, ctx context.Context, client *dbent.Client, suffix string) *dbent.PaymentOrder {
	t.Helper()

	user, err := client.User.Create().
		SetEmail(suffix + "@example.com").
		SetPasswordHash("hash").
		SetUsername(suffix).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName(suffix + "-provider").
		SetConfig("{}").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-" + suffix).
		SetOutTradeNo("sub2_" + suffix).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_" + suffix).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundPending).
		SetRefundAmount(100).
		SetRefundReason("pending refund").
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductBalance":true,"deductionType":"balance","balanceToDeduct":100,"deductionRollbackOK":true}`).
		Save(ctx)
	require.NoError(t, err)
	return order
}

func replacePaymentProviderFactoryForTest(t *testing.T, prov payment.Provider) func() {
	t.Helper()
	original := createPaymentProviderFromInstance
	createPaymentProviderFromInstance = func(providerKey, instanceID string, config map[string]string) (payment.Provider, error) {
		return prov, nil
	}
	return func() { createPaymentProviderFromInstance = original }
}

type refundProviderTestDouble struct{}

func (refundProviderTestDouble) Name() string { return "refund-test" }
func (refundProviderTestDouble) ProviderKey() string {
	return payment.TypeStripe
}
func (refundProviderTestDouble) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}
func (refundProviderTestDouble) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, nil
}
func (refundProviderTestDouble) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, nil
}
func (refundProviderTestDouble) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, nil
}
func (refundProviderTestDouble) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, nil
}

type refundCountingProviderTestDouble struct {
	refundProviderTestDouble
	refundCalls int
}

func (p *refundCountingProviderTestDouble) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	p.refundCalls++
	return &payment.RefundResponse{RefundID: "rf_counted", Status: payment.ProviderStatusPending}, nil
}

type unsafeRefundProviderTestDouble struct {
	refundProviderTestDouble
	refundCalls int
}

func (p *unsafeRefundProviderTestDouble) ProviderKey() string { return payment.TypeEasyPay }

func (p *unsafeRefundProviderTestDouble) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	p.refundCalls++
	return &payment.RefundResponse{RefundID: "unsafe", Status: payment.ProviderStatusPending}, nil
}

type refundQueryProviderTestDouble struct {
	refundProviderTestDouble
	refundResponse     *payment.RefundResponse
	queryErr           error
	refundCallResponse *payment.RefundResponse
	metadata           map[string]string
	queryCalls         int
	refundCalls        int
	queryContext       context.Context
	refundContext      context.Context
	refundContextErr   error
	lastQueryRequest   payment.RefundQueryRequest
	lastRefundRequest  payment.RefundRequest
}

type unsafeRefundQueryProviderTestDouble struct {
	refundQueryProviderTestDouble
}

type alipayRefundQueryProviderTestDouble struct {
	refundQueryProviderTestDouble
}

type wxpayRefundQueryProviderTestDouble struct {
	refundQueryProviderTestDouble
}

func (*alipayRefundQueryProviderTestDouble) ProviderKey() string { return payment.TypeAlipay }
func (*wxpayRefundQueryProviderTestDouble) ProviderKey() string  { return payment.TypeWxpay }

func (*unsafeRefundQueryProviderTestDouble) ProviderKey() string { return payment.TypeEasyPay }

func (p *refundQueryProviderTestDouble) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	p.queryCalls++
	p.queryContext = ctx
	p.lastQueryRequest = req
	return p.refundResponse, p.queryErr
}

func (p *refundQueryProviderTestDouble) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	p.refundCalls++
	p.refundContext = ctx
	p.refundContextErr = ctx.Err()
	p.lastRefundRequest = req
	return p.refundCallResponse, nil
}

func (p *refundQueryProviderTestDouble) MerchantIdentityMetadata() map[string]string {
	if p.metadata != nil {
		return p.metadata
	}
	return map[string]string{"currency": payment.DefaultPaymentCurrency}
}

type refundUserSubscriptionRepoStub struct {
	UserSubscriptionRepository
	active                     *UserSubscription
	deleted                    bool
	extendID                   int64
	extendDays                 int
	restoreCalls               int
	extendFailures             int
	getActiveCalls             int
	getByIDErr                 error
	getByIDIncludeDeletedErr   error
	getByIDIncludeDeletedCalls int
	onExtendExpiry             func(context.Context)
}

type refundFailingSubscriptionCache struct {
	BillingCache
	invalidateCalls int
}

func (c *refundFailingSubscriptionCache) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	c.invalidateCalls++
	return errors.New("injected subscription cache failure")
}

func (s *refundUserSubscriptionRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	s.getActiveCalls++
	if !s.deleted && s.active != nil && s.active.UserID == userID && s.active.GroupID == groupID {
		cp := *s.active
		return &cp, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (s *refundUserSubscriptionRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	if !s.deleted && s.active != nil && s.active.ID == id {
		cp := *s.active
		return &cp, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (s *refundUserSubscriptionRepoStub) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	return s.GetByID(ctx, id)
}

func (s *refundUserSubscriptionRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*UserSubscription, error) {
	s.getByIDIncludeDeletedCalls++
	if s.getByIDIncludeDeletedErr != nil {
		return nil, s.getByIDIncludeDeletedErr
	}
	if s.active == nil || s.active.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *s.active
	return &cp, nil
}

func (s *refundUserSubscriptionRepoStub) Delete(_ context.Context, id int64) error {
	if s.deleted || s.active == nil || s.active.ID != id {
		return ErrSubscriptionNotFound
	}
	now := time.Now()
	s.active.DeletedAt = &now
	s.deleted = true
	return nil
}

func (s *refundUserSubscriptionRepoStub) ExistsActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (bool, error) {
	return !s.deleted && s.active != nil && s.active.UserID == userID && s.active.GroupID == groupID, nil
}

func (s *refundUserSubscriptionRepoStub) Restore(_ context.Context, id int64, restoredStatus string) (*UserSubscription, error) {
	if !s.deleted || s.active == nil || s.active.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	s.restoreCalls++
	s.deleted = false
	s.active.DeletedAt = nil
	s.active.Status = restoredStatus
	cp := *s.active
	return &cp, nil
}

func (s *refundUserSubscriptionRepoStub) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	if s.deleted || s.active == nil || s.active.ID != subscriptionID {
		return ErrSubscriptionNotFound
	}
	if s.extendFailures > 0 {
		s.extendFailures--
		return errors.New("injected extend failure")
	}
	s.extendID = subscriptionID
	s.extendDays = int(newExpiresAt.Sub(s.active.ExpiresAt).Hours() / 24)
	s.active.ExpiresAt = newExpiresAt
	if s.onExtendExpiry != nil {
		s.onExtendExpiry(ctx)
	}
	return nil
}

func (s *refundUserSubscriptionRepoStub) UpdateStatus(_ context.Context, subscriptionID int64, status string) error {
	if s.deleted || s.active == nil || s.active.ID != subscriptionID {
		return ErrSubscriptionNotFound
	}
	s.active.Status = status
	return nil
}
