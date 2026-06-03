package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// UserGroupRateLimitWindowRecord represents a user's 5-hour USD usage window for one group.
type UserGroupRateLimitWindowRecord struct {
	UserID        int64
	GroupID       int64
	GroupName     string
	RateLimit5h   float64
	Usage5hUSD    float64
	Window5hStart *time.Time
}

func (r UserGroupRateLimitWindowRecord) Window5hResetAt() *time.Time {
	if r.Window5hStart == nil || IsWindowExpired(r.Window5hStart, RateLimitWindow5h) {
		return nil
	}
	resetAt := r.Window5hStart.Add(RateLimitWindow5h)
	return &resetAt
}

func (r UserGroupRateLimitWindowRecord) EffectiveUsage5hUSD() float64 {
	if IsWindowExpired(r.Window5hStart, RateLimitWindow5h) {
		return 0
	}
	return r.Usage5hUSD
}

// UserGroupRateLimitWindowRepository stores per-user, per-group 5-hour usage windows.
type UserGroupRateLimitWindowRepository interface {
	Get(ctx context.Context, userID, groupID int64) (*UserGroupRateLimitWindowRecord, error)
	ListByUser(ctx context.Context, userID int64) ([]UserGroupRateLimitWindowRecord, error)
	ListByGroup(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]UserGroupRateLimitWindowRecord, *pagination.PaginationResult, error)
	IncrementWithWindowReset(ctx context.Context, userID, groupID int64, cost float64, now time.Time) error
	Reset(ctx context.Context, userID, groupID int64) (*UserGroupRateLimitWindowRecord, error)
}
