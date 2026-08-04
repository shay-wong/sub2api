//go:build unit

package provider

import (
	"bytes"
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"
)

func TestStripeQueryRefundRequiresRefundID(t *testing.T) {
	prov, err := NewStripe("stripe-test", map[string]string{
		"secretKey": "sk_test",
		"currency":  "USD",
	})
	require.NoError(t, err)

	resp, err := prov.QueryRefund(context.Background(), payment.RefundQueryRequest{
		TradeNo: "pi_test",
	})

	require.Nil(t, resp)
	require.ErrorContains(t, err, "missing refund id")
}

type stripeRefundBackend struct {
	params []*stripe.RefundCreateParams
	status stripe.RefundStatus
}

func (b *stripeRefundBackend) Call(_ string, _ string, _ string, params stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	b.params = append(b.params, params.(*stripe.RefundCreateParams))
	refund := v.(*stripe.Refund)
	refund.ID = "re_123"
	refund.Status = b.status
	if refund.Status == "" {
		refund.Status = stripe.RefundStatusSucceeded
	}
	return nil
}

func TestStripeRefundCreateResponseUsesSharedStatusMapping(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status stripe.RefundStatus
		want   string
	}{
		{name: "succeeded", status: stripe.RefundStatusSucceeded, want: payment.ProviderStatusSuccess},
		{name: "failed", status: stripe.RefundStatusFailed, want: payment.ProviderStatusFailed},
		{name: "canceled", status: stripe.RefundStatusCanceled, want: payment.ProviderStatusFailed},
		{name: "pending", status: stripe.RefundStatusPending, want: payment.ProviderStatusPending},
		{name: "unknown", status: stripe.RefundStatus("future_status"), want: payment.ProviderStatusPending},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := &stripeRefundBackend{status: tt.status}
			provider := &Stripe{
				config:      map[string]string{"currency": "CNY"},
				initialized: true,
				sc:          stripe.NewClient("sk_test", stripe.WithBackends(&stripe.Backends{API: backend})),
			}

			resp, err := provider.Refund(context.Background(), payment.RefundRequest{
				TradeNo: "pi_123",
				OrderID: "sub2_order_456",
				Amount:  "12.34",
			})

			require.NoError(t, err)
			require.Equal(t, tt.want, resp.Status)
		})
	}
}

func (*stripeRefundBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) CallRaw(string, string, string, []byte, *stripe.Params, stripe.LastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) SetMaxNetworkRetries(int64) {}

func TestStripeRefundUsesStableAmountSpecificIdempotencyKey(t *testing.T) {
	backend := &stripeRefundBackend{}
	client := stripe.NewClient("sk_test", stripe.WithBackends(&stripe.Backends{API: backend}))
	provider := &Stripe{
		config:      map[string]string{"currency": "CNY"},
		initialized: true,
		sc:          client,
	}

	refund := func(amount string) {
		_, err := provider.Refund(context.Background(), payment.RefundRequest{
			TradeNo: "pi_123",
			OrderID: "sub2_order_456",
			Amount:  amount,
		})
		require.NoError(t, err)
	}

	refund("12.34")
	refund("12.34")
	refund("12.35")

	require.Len(t, backend.params, 3)
	require.Equal(t, int64(1234), *backend.params[0].Amount)
	require.Equal(t, "re-sub2_order_456-1234", *backend.params[0].IdempotencyKey)
	require.Equal(t, backend.params[0].IdempotencyKey, backend.params[1].IdempotencyKey)
	require.Equal(t, int64(1235), *backend.params[2].Amount)
	require.Equal(t, "re-sub2_order_456-1235", *backend.params[2].IdempotencyKey)
	require.NotEqual(t, *backend.params[0].IdempotencyKey, *backend.params[2].IdempotencyKey)
}

func TestStripeRefundUsesProviderRequestIDAcrossRecovery(t *testing.T) {
	backend := &stripeRefundBackend{}
	client := stripe.NewClient("sk_test", stripe.WithBackends(&stripe.Backends{API: backend}))
	provider := &Stripe{config: map[string]string{"currency": "CNY"}, initialized: true, sc: client}

	for _, requestID := range []string{"attempt-1", "attempt-1", "attempt-2"} {
		_, err := provider.Refund(context.Background(), payment.RefundRequest{
			TradeNo:           "pi_123",
			OrderID:           "sub2_order_456",
			ProviderRequestID: requestID,
			Amount:            "12.34",
		})
		require.NoError(t, err)
	}

	require.Equal(t, "re-attempt-1", *backend.params[0].IdempotencyKey)
	require.Equal(t, backend.params[0].IdempotencyKey, backend.params[1].IdempotencyKey)
	require.Equal(t, "re-attempt-2", *backend.params[2].IdempotencyKey)
}
