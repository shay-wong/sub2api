package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIProxyStreamCircuitThresholdTTLAndSuccessReset(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 2,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       16,
	})

	tripped, _ := circuit.recordFailure(1, base)
	require.False(t, tripped)
	require.False(t, circuit.isBlocked(1, base))
	require.True(t, circuit.recordSuccess(1))

	tripped, _ = circuit.recordFailure(1, base.Add(10*time.Second))
	require.False(t, tripped, "success must clear the previous failure observation")
	tripped, until := circuit.recordFailure(1, base.Add(20*time.Second))
	require.True(t, tripped)
	require.Equal(t, base.Add(20*time.Second+10*time.Minute), until)
	require.True(t, circuit.recordSuccess(1), "a completed stream must clear an active quarantine")
	require.False(t, circuit.isBlocked(1, base.Add(30*time.Second)))

	tripped, _ = circuit.recordFailure(2, base)
	require.False(t, tripped)
	tripped, _ = circuit.recordFailure(2, base.Add(2*time.Minute))
	require.False(t, tripped, "failures outside the window must not accumulate")
}

func TestOpenAIProxyStreamCircuitCollapsesBurstFailures(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 2,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		collapseInterval: 3 * time.Second,
		maxEntries:       16,
	})

	// One HTTP/2 connection loss kills several multiplexed streams at once:
	// the near-simultaneous reports must count as a single failure event.
	tripped, _ := circuit.recordFailure(1, base)
	require.False(t, tripped)
	tripped, _ = circuit.recordFailure(1, base.Add(time.Second))
	require.False(t, tripped, "burst failures inside the collapse interval must merge")
	tripped, _ = circuit.recordFailure(1, base.Add(2*time.Second))
	require.False(t, tripped, "burst failures inside the collapse interval must merge")
	require.False(t, circuit.isBlocked(1, base.Add(2*time.Second)))

	// A second, distinct incident past the collapse interval still trips.
	tripped, _ = circuit.recordFailure(1, base.Add(5*time.Second))
	require.True(t, tripped)
	require.True(t, circuit.isBlocked(1, base.Add(5*time.Second)))
}

func TestOpenAIProxyStreamCircuitDisabled(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		disabled:         true,
		failureThreshold: 1,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       16,
	})

	tripped, _ := circuit.recordFailure(1, base)
	require.False(t, tripped)
	require.False(t, circuit.isBlocked(1, base))
}

func TestOpenAIProxyStreamQuarantineBypassContext(t *testing.T) {
	proxyID := int64(7)
	account := &Account{ID: 1, Platform: PlatformOpenAI, ProxyID: &proxyID}
	svc := &OpenAIGatewayService{}
	svc.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 1,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       16,
	})
	svc.openaiProxyStreamCircuit.recordFailure(proxyID, time.Now())

	ctx := context.Background()
	observedCtx, observation := withOpenAIProxyStreamQuarantineObservation(ctx)
	require.True(t, svc.isOpenAIProxyStreamQuarantined(observedCtx, account))
	require.True(t, svc.isOpenAIProxyStreamQuarantined(observedCtx, account))
	excludedAccounts, blockedProxies := observation.counts()
	require.Equal(t, 1, excludedAccounts)
	require.Equal(t, 1, blockedProxies)
	require.False(t, svc.isOpenAIProxyStreamQuarantined(withOpenAIProxyStreamQuarantineBypass(ctx), account))
	svc.clearOpenAIProxyStreamDisconnect(account)
	require.False(t, svc.isOpenAIProxyStreamQuarantined(ctx, account), "a completed stream must clear the active quarantine")
}

func TestOpenAIProxyStreamCircuitBoundsEntries(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 1,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       2,
	})

	circuit.recordFailure(1, base)
	circuit.recordFailure(2, base.Add(time.Second))
	circuit.recordFailure(3, base.Add(2*time.Second))

	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	require.Len(t, circuit.entries, 2)
	_, oldestRetained := circuit.entries[1]
	require.False(t, oldestRetained, "the oldest entry must be evicted at the bound")
}
