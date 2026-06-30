package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type rateLimitGeminiUsageRepo struct {
	UsageLogRepository
	calls          []usagestats.UsageLogFilters
	sources        []string
	stats          []usagestats.ModelStat
	batchCalls     [][]int64
	batchResponses map[int64]GeminiUsageTotals
}

func (r *rateLimitGeminiUsageRepo) GetModelStatsWithUsageFiltersBySource(ctx context.Context, startTime, endTime time.Time, filters usagestats.UsageLogFilters, source string) ([]usagestats.ModelStat, error) {
	r.calls = append(r.calls, filters)
	r.sources = append(r.sources, source)
	return r.stats, nil
}

func (r *rateLimitGeminiUsageRepo) GetGeminiUsageTotalsBatch(ctx context.Context, accountIDs []int64, startTime, endTime time.Time) (map[int64]GeminiUsageTotals, error) {
	ids := append([]int64(nil), accountIDs...)
	r.batchCalls = append(r.batchCalls, ids)
	out := make(map[int64]GeminiUsageTotals, len(accountIDs))
	for _, accountID := range accountIDs {
		out[accountID] = r.batchResponses[accountID]
	}
	return out, nil
}

func TestRateLimitServicePreCheckGeminiUsesUpstreamModelStatsForAccount(t *testing.T) {
	t.Parallel()

	repo := &rateLimitGeminiUsageRepo{
		stats: []usagestats.ModelStat{
			{Model: "gemini-2.5-pro", Requests: 2, TotalTokens: 20, ActualCost: 0.02},
			{Model: "gemini-2.5-flash", Requests: 3, TotalTokens: 30, ActualCost: 0.03},
		},
	}
	svc := NewRateLimitService(nil, repo, nil, NewGeminiQuotaService(nil, nil), nil)
	account := &Account{
		ID:       88,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"oauth_type": "google_one",
			"tier_id":    GeminiTierGoogleOneFree,
		},
	}

	ok, err := svc.PreCheckUsage(context.Background(), account, "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("PreCheckUsage() error = %v", err)
	}
	if !ok {
		t.Fatal("expected account to stay eligible below shared quota")
	}
	if len(repo.calls) != 2 {
		t.Fatalf("expected daily and minute stats calls, got %d", len(repo.calls))
	}
	for i, filters := range repo.calls {
		if filters.AccountID != account.ID {
			t.Fatalf("call %d AccountID = %d, want %d", i, filters.AccountID, account.ID)
		}
		if repo.sources[i] != usagestats.ModelSourceUpstream {
			t.Fatalf("call %d source = %q, want upstream", i, repo.sources[i])
		}
	}
}

func TestRateLimitServicePreCheckGeminiClassifiesMappedUpstreamModel(t *testing.T) {
	t.Parallel()

	repo := &rateLimitGeminiUsageRepo{
		stats: []usagestats.ModelStat{
			{Model: "gemini-2.5-flash", Requests: 15, TotalTokens: 150, ActualCost: 0.15},
		},
	}
	svc := NewRateLimitService(nil, repo, nil, NewGeminiQuotaService(nil, nil), nil)
	account := &Account{
		ID:       89,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "test-key",
			"tier_id": GeminiTierAIStudioFree,
			"model_mapping": map[string]any{
				"my-fast-alias": "gemini-2.5-flash",
			},
		},
	}

	ok, err := svc.PreCheckUsage(context.Background(), account, "my-fast-alias")
	if err != nil {
		t.Fatalf("PreCheckUsage() error = %v", err)
	}
	if ok {
		t.Fatal("expected mapped flash alias to hit flash RPM quota")
	}
	for i, source := range repo.sources {
		if source != usagestats.ModelSourceUpstream {
			t.Fatalf("call %d source = %q, want upstream", i, source)
		}
	}
}

func TestRateLimitServicePreCheckUsageBatchUsesGeminiBatchProvider(t *testing.T) {
	t.Parallel()

	repo := &rateLimitGeminiUsageRepo{
		batchResponses: map[int64]GeminiUsageTotals{
			91: {ProRequests: 999, FlashRequests: 1},
			92: {ProRequests: 2, FlashRequests: 3},
		},
	}
	svc := NewRateLimitService(nil, repo, nil, NewGeminiQuotaService(nil, nil), nil)
	accounts := []*Account{
		{
			ID:       91,
			Platform: PlatformGemini,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"oauth_type": "google_one",
				"tier_id":    GeminiTierGoogleOneFree,
			},
		},
		{
			ID:       92,
			Platform: PlatformGemini,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"oauth_type": "google_one",
				"tier_id":    GeminiTierGoogleOneFree,
			},
		},
	}

	got, err := svc.PreCheckUsageBatch(context.Background(), accounts, "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("PreCheckUsageBatch() error = %v", err)
	}
	if got[91] {
		t.Fatal("expected account 91 to be skipped after shared daily quota reaches limit")
	}
	if !got[92] {
		t.Fatal("expected account 92 to stay eligible")
	}
	if len(repo.batchCalls) != 2 {
		t.Fatalf("expected daily and minute batch provider calls, got %d", len(repo.batchCalls))
	}
	if len(repo.calls) != 0 {
		t.Fatalf("expected no per-account fallback stats calls, got %d", len(repo.calls))
	}
}

func TestRateLimitServicePreCheckUsageBatchClassifiesMappedUpstreamModel(t *testing.T) {
	t.Parallel()

	repo := &rateLimitGeminiUsageRepo{
		batchResponses: map[int64]GeminiUsageTotals{
			93: {FlashRequests: 15},
		},
	}
	svc := NewRateLimitService(nil, repo, nil, NewGeminiQuotaService(nil, nil), nil)
	accounts := []*Account{{
		ID:       93,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "test-key",
			"tier_id": GeminiTierAIStudioFree,
			"model_mapping": map[string]any{
				"my-fast-alias": "gemini-2.5-flash",
			},
		},
	}}

	got, err := svc.PreCheckUsageBatch(context.Background(), accounts, "my-fast-alias")
	if err != nil {
		t.Fatalf("PreCheckUsageBatch() error = %v", err)
	}
	if got[93] {
		t.Fatal("expected mapped flash alias to hit flash RPM quota")
	}
	if len(repo.batchCalls) != 2 {
		t.Fatalf("expected daily and minute batch provider calls, got %d", len(repo.batchCalls))
	}
	if len(repo.calls) != 0 {
		t.Fatalf("expected no per-account fallback stats calls, got %d", len(repo.calls))
	}
}
