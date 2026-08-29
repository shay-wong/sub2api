package handler

import (
	"context"
	"net/http"
	"strings"
	"sync"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"golang.org/x/sync/semaphore"
)

const (
	openAIRequestMemoryAmplification int64 = 8
	defaultOpenAIRequestMemoryBudget int64 = 256 << 20
)

type openAIRequestMemoryLimiter struct {
	budget int64
	sem    *semaphore.Weighted
}

func newOpenAIRequestMemoryLimiter(budget int64) *openAIRequestMemoryLimiter {
	if budget <= 0 {
		budget = defaultOpenAIRequestMemoryBudget
	}
	return &openAIRequestMemoryLimiter{budget: budget, sem: semaphore.NewWeighted(budget)}
}

func (l *openAIRequestMemoryLimiter) Acquire(ctx context.Context, req *http.Request) (func(), error) {
	weight := l.requestWeight(req)
	if weight == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := l.sem.Acquire(ctx, weight); err != nil {
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() { l.sem.Release(weight) })
	}, nil
}

func (l *openAIRequestMemoryLimiter) requestWeight(req *http.Request) int64 {
	if l == nil || l.sem == nil || l.budget <= 0 || req == nil {
		return 0
	}
	encoding := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if req.ContentLength < 0 || len(req.TransferEncoding) > 0 || (encoding != "" && encoding != "identity") {
		return l.budget
	}
	if req.ContentLength < pkghttputil.DefaultDiskSpillThreshold {
		return 0
	}
	if req.ContentLength >= l.budget/openAIRequestMemoryAmplification {
		return l.budget
	}
	return req.ContentLength * openAIRequestMemoryAmplification
}
