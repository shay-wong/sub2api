package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const liveRecoveryBatchSize int64 = 100

type LiveCallRecoveryService struct {
	gateway  *OpenAIGatewayService
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewLiveCallRecoveryService(gateway *OpenAIGatewayService, interval time.Duration) *LiveCallRecoveryService {
	ctx, cancel := context.WithCancel(context.Background())
	return &LiveCallRecoveryService{
		gateway:  gateway,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (s *LiveCallRecoveryService) Start() {
	if s == nil || s.gateway == nil || s.interval <= 0 {
		return
	}
	if _, err := s.gateway.liveStore(); err != nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *LiveCallRecoveryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(s.cancel)
	s.wg.Wait()
}

func (s *LiveCallRecoveryService) runOnce() {
	if s == nil || s.gateway == nil || s.ctx.Err() != nil {
		return
	}
	store, err := s.gateway.liveStore()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, liveRedisOperationTimeout)
	records, err := store.ListRecoverableLiveCalls(ctx, time.Now(), liveRecoveryBatchSize)
	cancel()
	if err != nil {
		logger.FromContext(context.Background()).Error("openai_live_recovery_list_failed", zap.Error(err))
		return
	}
	for _, record := range records {
		if s.ctx.Err() != nil {
			return
		}
		if record.Provisional {
			s.gateway.closeLiveCreateIntent(record)
			continue
		}
		if !s.gateway.tryFinalizeLiveCall(s.ctx, record) {
			if s.ctx.Err() != nil {
				return
			}
			s.gateway.scheduleLiveCallRecovery(record)
		}
	}
}
