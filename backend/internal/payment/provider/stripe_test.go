package provider

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
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
