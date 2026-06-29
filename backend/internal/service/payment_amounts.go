package service

import (
	"math"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const (
	defaultPaymentMultiplier         = 1.0
	defaultBalanceRechargeMultiplier = defaultPaymentMultiplier
)

func normalizePaymentMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return defaultPaymentMultiplier
	}
	return multiplier
}

func normalizeBalanceRechargeMultiplier(multiplier float64) float64 {
	return normalizePaymentMultiplier(multiplier)
}

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
}

func calculateGatewayPaymentAmount(orderAmount, multiplier float64, currency string) float64 {
	fractionDigits := int32(payment.CurrencyMaxFractionDigits(currency))
	return decimal.NewFromFloat(orderAmount).
		Div(decimal.NewFromFloat(normalizePaymentMultiplier(multiplier))).
		Round(fractionDigits).
		InexactFloat64()
}

func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount float64, currency string) float64 {
	if orderAmount <= 0 || payAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	fractionDigits := int32(payment.CurrencyMaxFractionDigits(currency))
	if math.Abs(refundAmount-orderAmount) <= paymentAmountToleranceForCurrency(currency) {
		return decimal.NewFromFloat(payAmount).Round(fractionDigits).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(fractionDigits).
		InexactFloat64()
}
