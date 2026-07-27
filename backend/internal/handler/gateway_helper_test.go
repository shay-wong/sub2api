package handler

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestWrapReleaseOnDone_NoGoroutineLeak 验证 wrapReleaseOnDone 修复后不会泄露 goroutine
func TestWrapReleaseOnDone_NoGoroutineLeak(t *testing.T) {
	// 记录测试开始时的 goroutine 数量
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var releaseCount int32
	release := wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 正常释放
	release()

	// 等待足够时间确保 goroutine 退出
	time.Sleep(200 * time.Millisecond)

	// 验证只释放一次
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}

	// 强制 GC，清理已退出的 goroutine
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// 验证 goroutine 数量没有增加（允许±2的误差，考虑到测试框架本身可能创建的 goroutine）
	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("goroutine leak detected: initial=%d, final=%d, leaked=%d",
			initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)
	}
}

// TestWrapReleaseOnDone_ContextCancellation 验证 context 取消时也能正确释放
func TestWrapReleaseOnDone_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var releaseCount int32
	_ = wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 取消 context，应该触发释放
	cancel()

	// 等待释放完成
	time.Sleep(100 * time.Millisecond)

	// 验证释放被调用
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}
}

func TestWrapReleaseOnDone_AlreadyCancelledReleasesExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var releaseCount int32
	release := wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})
	release()

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&releaseCount) == 1
	}, time.Second, time.Millisecond)
}

// Streaming forwarders keep draining upstream usage after the client leaves, so
// the account slot must remain held until the forward call explicitly returns.
func TestWrapAccountReleaseOnForwardDone_StreamingWaitsForForwardCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	released := make(chan struct{})
	release := wrapAccountReleaseOnForwardDone(ctx, true, func() { close(released) })

	cancel()
	select {
	case <-released:
		t.Fatal("streaming account slot released on client cancellation")
	case <-time.After(20 * time.Millisecond):
	}

	release()
	require.Eventually(t, func() bool {
		select {
		case <-released:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}

func TestAcquireResponsesAccountSlot_StreamingReleaseWaitsForForwardCompletion(t *testing.T) {
	newContext := func(t *testing.T) (*gin.Context, context.CancelFunc) {
		t.Helper()
		c, _ := newHelperTestContext("POST", "/openai/v1/responses")
		ctx, cancel := context.WithCancel(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		return c, cancel
	}
	cacheReleaseCount := func(cache *helperConcurrencyCacheStub) int {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.accountReleaseCalls
	}
	assertHeld := func(t *testing.T, cancel context.CancelFunc, release func(), released func() int) {
		t.Helper()
		cancel()
		time.Sleep(20 * time.Millisecond)
		require.Zero(t, released(), "streaming slot released before forward returned")
		release()
		require.Eventually(t, func() bool { return released() == 1 }, time.Second, time.Millisecond)
	}

	t.Run("scheduler acquired", func(t *testing.T) {
		c, cancel := newContext(t)
		var releases atomic.Int32
		h := &OpenAIGatewayHandler{}
		release, acquired := h.acquireResponsesAccountSlot(c, nil, "", &service.AccountSelectionResult{
			Account:     &service.Account{ID: 101},
			Acquired:    true,
			ReleaseFunc: func() { releases.Add(1) },
		}, true, new(bool), zap.NewNop())
		require.True(t, acquired)
		assertHeld(t, cancel, release, func() int { return int(releases.Load()) })
	})

	t.Run("fast acquired", func(t *testing.T) {
		c, cancel := newContext(t)
		cache := &helperConcurrencyCacheStub{accountSeq: []bool{true}}
		h := &OpenAIGatewayHandler{
			gatewayService:    &service.OpenAIGatewayService{},
			concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Millisecond),
		}
		release, acquired := h.acquireResponsesAccountSlot(c, nil, "", &service.AccountSelectionResult{
			Account:  &service.Account{ID: 102},
			WaitPlan: &service.AccountWaitPlan{MaxConcurrency: 1, Timeout: time.Second, MaxWaiting: 1},
		}, true, new(bool), zap.NewNop())
		require.True(t, acquired)
		assertHeld(t, cancel, release, func() int { return cacheReleaseCount(cache) })
	})

	t.Run("waited acquired", func(t *testing.T) {
		c, cancel := newContext(t)
		cache := &helperConcurrencyCacheStub{accountSeq: []bool{false, true}}
		h := &OpenAIGatewayHandler{
			gatewayService:    &service.OpenAIGatewayService{},
			concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Millisecond),
		}
		release, acquired := h.acquireResponsesAccountSlot(c, nil, "", &service.AccountSelectionResult{
			Account:  &service.Account{ID: 103},
			WaitPlan: &service.AccountWaitPlan{MaxConcurrency: 1, Timeout: time.Second, MaxWaiting: 1},
		}, true, new(bool), zap.NewNop())
		require.True(t, acquired)
		assertHeld(t, cancel, release, func() int { return cacheReleaseCount(cache) })
	})
}

func TestAcquireResponsesAccountSlot_NonStreamingReleaseFollowsClientCancellation(t *testing.T) {
	c, _ := newHelperTestContext("POST", "/openai/v1/responses")
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	var releases atomic.Int32
	h := &OpenAIGatewayHandler{}
	release, acquired := h.acquireResponsesAccountSlot(c, nil, "", &service.AccountSelectionResult{
		Account:     &service.Account{ID: 104},
		Acquired:    true,
		ReleaseFunc: func() { releases.Add(1) },
	}, false, new(bool), zap.NewNop())
	require.True(t, acquired)
	defer release()

	cancel()
	require.Eventually(t, func() bool { return releases.Load() == 1 }, time.Second, time.Millisecond)
}

// TestWrapReleaseOnDone_MultipleCallsOnlyReleaseOnce 验证多次调用 release 只释放一次
func TestWrapReleaseOnDone_MultipleCallsOnlyReleaseOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var releaseCount int32
	release := wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 调用多次
	release()
	release()
	release()

	// 等待执行完成
	time.Sleep(100 * time.Millisecond)

	// 验证只释放一次
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}
}

// TestWrapReleaseOnDone_NilReleaseFunc 验证 nil releaseFunc 不会 panic
func TestWrapReleaseOnDone_NilReleaseFunc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := wrapReleaseOnDone(ctx, nil)

	if release != nil {
		t.Error("expected nil release function when releaseFunc is nil")
	}
}

// TestWrapReleaseOnDone_ConcurrentCalls 验证并发调用的安全性
func TestWrapReleaseOnDone_ConcurrentCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var releaseCount int32
	release := wrapReleaseOnDone(ctx, func() {
		atomic.AddInt32(&releaseCount, 1)
	})

	// 并发调用 release
	const numGoroutines = 10
	for i := 0; i < numGoroutines; i++ {
		go release()
	}

	// 等待所有 goroutine 完成
	time.Sleep(200 * time.Millisecond)

	// 验证只释放一次
	if count := atomic.LoadInt32(&releaseCount); count != 1 {
		t.Errorf("expected release count to be 1, got %d", count)
	}
}

// BenchmarkWrapReleaseOnDone 性能基准测试
func BenchmarkWrapReleaseOnDone(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		release := wrapReleaseOnDone(ctx, func() {})
		release()
	}
}
