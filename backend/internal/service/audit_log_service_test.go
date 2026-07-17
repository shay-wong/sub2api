package service

import (
	"context"
	"errors"
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
	nextID       int64
	ids          []int64
	clearID      int64
	nextIDErr    error
}

func (r *auditLogBarrierRepo) NextID(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nextIDErr != nil {
		return 0, r.nextIDErr
	}
	r.nextID++
	return r.nextID, nil
}

func TestAuditLogServiceRecordDropsEntryWhenIDReservationFails(t *testing.T) {
	repo := &auditLogBarrierRepo{nextIDErr: errors.New("database unavailable")}
	svc := NewAuditLogService(repo, nil)

	svc.Record(&AuditLog{Action: "not-queued"})

	if svc.droppedCount != 1 {
		t.Fatalf("droppedCount = %d, want 1", svc.droppedCount)
	}
	if len(svc.queue) != 0 {
		t.Fatalf("queue length = %d, want 0", len(svc.queue))
	}
}

func (r *auditLogBarrierRepo) BatchInsert(_ context.Context, logs []*AuditLog) (int64, error) {
	r.flushStarted <- struct{}{}
	<-r.releaseFlush
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, log := range logs {
		r.events = append(r.events, "batch:"+log.Action)
		r.ids = append(r.ids, log.ID)
	}
	return int64(len(logs)), nil
}

func (r *auditLogBarrierRepo) ClearAll(_ context.Context, trace *AuditLog) (int64, error) {
	r.mu.Lock()
	r.events = append(r.events, "clear")
	r.nextID++
	trace.ID = r.nextID
	r.clearID = trace.ID
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
	if len(repo.ids) != 1 || repo.ids[0] != 1 || repo.clearID != 2 {
		t.Fatalf("ids = %v clear=%d, want [1] clear=2", repo.ids, repo.clearID)
	}
}
