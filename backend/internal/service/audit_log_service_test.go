package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

type auditLogBarrierRepo struct {
	mu           sync.Mutex
	events       []string
	flushStarted chan struct{}
	releaseFlush chan struct{}
	clearCalled  chan struct{}
}

func (r *auditLogBarrierRepo) BatchInsert(_ context.Context, logs []*AuditLog) (int64, error) {
	r.flushStarted <- struct{}{}
	<-r.releaseFlush
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, log := range logs {
		r.events = append(r.events, "batch:"+log.Action)
	}
	return int64(len(logs)), nil
}

func (r *auditLogBarrierRepo) ClearAll(_ context.Context, _ *AuditLog) (int64, error) {
	r.mu.Lock()
	r.events = append(r.events, "clear")
	r.mu.Unlock()
	close(r.clearCalled)
	return 1, nil
}

func (r *auditLogBarrierRepo) List(context.Context, *AuditLogFilter) (*AuditLogList, error) {
	return nil, nil
}

func (r *auditLogBarrierRepo) GetByID(context.Context, int64) (*AuditLog, error) {
	return nil, nil
}

func (r *auditLogBarrierRepo) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestAuditLogServiceClearAllWaitsForPreClearQueue(t *testing.T) {
	repo := &auditLogBarrierRepo{
		flushStarted: make(chan struct{}, 1),
		releaseFlush: make(chan struct{}),
		clearCalled:  make(chan struct{}),
	}
	service := NewAuditLogService(repo, nil)
	service.Start()
	defer service.Stop()

	service.Record(&AuditLog{Action: "before"})
	result := make(chan error, 1)
	go func() {
		_, err := service.ClearAll(context.Background(), &AuditLog{})
		result <- err
	}()

	select {
	case <-repo.flushStarted:
	case <-time.After(time.Second):
		t.Fatal("pre-clear queue was not flushed")
	}
	select {
	case <-repo.clearCalled:
		t.Fatal("clear ran before the pre-clear queue flush completed")
	default:
	}
	close(repo.releaseFlush)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("ClearAll() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ClearAll did not complete")
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.events) != 2 || repo.events[0] != "batch:before" || repo.events[1] != "clear" {
		t.Fatalf("event order = %v, want [batch:before clear]", repo.events)
	}
}
