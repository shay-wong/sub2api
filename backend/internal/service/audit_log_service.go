package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	auditLogQueueCapacity = 4096
	auditLogBatchSize     = 100
	auditLogFlushInterval = time.Second

	auditRetentionCheckInterval = 24 * time.Hour
	auditRetentionStartupDelay  = 5 * time.Minute
	auditRetentionBatchSize     = 5000
)

// AuditLogService 管理面操作审计日志服务。
// 写入端为非阻塞异步批量落库（不拖慢管理请求）；
// 读取端提供分页查询；清空端点由 handler 层做 TOTP 强校验后调用 ClearAll。
type AuditLogService struct {
	repo           AuditLogRepository
	settingService *SettingService

	queue chan auditLogQueueItem

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	droppedCount uint64
	writeFailed  uint64
	writtenCount uint64
}

type auditLogQueueItem struct {
	entry *AuditLog
	clear *auditLogClearRequest
}

type auditLogClearRequest struct {
	ctx    context.Context
	trace  *AuditLog
	result chan auditLogClearResult
}

type auditLogClearResult struct {
	deleted int64
	err     error
}

func NewAuditLogService(repo AuditLogRepository, settingService *SettingService) *AuditLogService {
	ctx, cancel := context.WithCancel(context.Background())
	return &AuditLogService{
		repo:           repo,
		settingService: settingService,
		queue:          make(chan auditLogQueueItem, auditLogQueueCapacity),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start 启动异步写入与保留期清理协程。
func (s *AuditLogService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.wg.Add(2)
	go s.runWriter()
	go s.runRetentionLoop()
}

// Stop 停止服务并尽量落盘队列中剩余记录。
func (s *AuditLogService) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

// Record 非阻塞入队一条审计记录；队列打满时丢弃并计数（管理面流量下几乎不可能发生）。
func (s *AuditLogService) Record(entry *AuditLog) {
	if s == nil || entry == nil {
		return
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	select {
	case <-s.ctx.Done():
		return
	default:
	}
	select {
	case s.queue <- auditLogQueueItem{entry: entry}:
	default:
		atomic.AddUint64(&s.droppedCount, 1)
	}
}

// List 分页查询审计日志。
func (s *AuditLogService) List(ctx context.Context, filter *AuditLogFilter) (*AuditLogList, error) {
	return s.repo.List(ctx, filter)
}

// GetByID 查询单条详情。
func (s *AuditLogService) GetByID(ctx context.Context, id int64) (*AuditLog, error) {
	return s.repo.GetByID(ctx, id)
}

// ClearAll 全量清空审计日志并写入留痕记录。
// 调用方（handler）必须先完成 TOTP 验证；本方法负责：
//  1. 等待此前已入队的记录落库
//  2. 在同一事务中统计、清空全表并写入 "audit_log.clear" 留痕记录
func (s *AuditLogService) ClearAll(ctx context.Context, trace *AuditLog) (int64, error) {
	if trace != nil {
		trace.Action = AuditActionAuditLogClear
		if trace.CreatedAt.IsZero() {
			trace.CreatedAt = time.Now().UTC()
		}
		if trace.Extra == nil {
			trace.Extra = map[string]any{}
		}
	}

	result := make(chan auditLogClearResult, 1)
	request := &auditLogClearRequest{ctx: ctx, trace: trace, result: result}
	select {
	case s.queue <- auditLogQueueItem{clear: request}:
	case <-ctx.Done():
		return 0, fmt.Errorf("queue audit log clear: %w", ctx.Err())
	case <-s.ctx.Done():
		return 0, fmt.Errorf("queue audit log clear: %w", context.Canceled)
	}

	res := <-result
	return res.deleted, res.err
}

func (s *AuditLogService) runWriter() {
	defer s.wg.Done()

	ticker := time.NewTicker(auditLogFlushInterval)
	defer ticker.Stop()

	batch := make([]*AuditLog, 0, auditLogBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		inserted, err := s.repo.BatchInsert(ctx, batch)
		cancel()
		if err != nil {
			atomic.AddUint64(&s.writeFailed, uint64(len(batch)))
			_, _ = fmt.Fprintf(os.Stderr, "time=%s level=WARN msg=\"audit log flush failed\" err=%v batch=%d\n",
				time.Now().Format(time.RFC3339Nano), err, len(batch))
		} else {
			atomic.AddUint64(&s.writtenCount, uint64(inserted))
		}
		batch = batch[:0]
		return err
	}
	handle := func(item auditLogQueueItem) {
		if item.clear != nil {
			if err := flush(); err != nil {
				item.clear.result <- auditLogClearResult{err: fmt.Errorf("flush audit logs before clear: %w", err)}
				return
			}
			deleted, err := s.repo.ClearAll(item.clear.ctx, item.clear.trace)
			if err != nil {
				err = fmt.Errorf("clear audit logs: %w", err)
			}
			item.clear.result <- auditLogClearResult{deleted: deleted, err: err}
			return
		}
		if item.entry == nil {
			return
		}
		batch = append(batch, item.entry)
		if len(batch) >= auditLogBatchSize {
			_ = flush()
		}
	}

	for {
		select {
		case <-s.ctx.Done():
			// 停机前排空队列。
			for {
				select {
				case item := <-s.queue:
					handle(item)
				default:
					_ = flush()
					return
				}
			}
		case item := <-s.queue:
			handle(item)
		case <-ticker.C:
			_ = flush()
		}
	}
}

// runRetentionLoop 按保留期定期删除过期审计日志。
// 删除操作幂等，多实例并发执行无害，因此无需选主。
func (s *AuditLogService) runRetentionLoop() {
	defer s.wg.Done()

	startupTimer := time.NewTimer(auditRetentionStartupDelay)
	defer startupTimer.Stop()
	select {
	case <-s.ctx.Done():
		return
	case <-startupTimer.C:
	}

	ticker := time.NewTicker(auditRetentionCheckInterval)
	defer ticker.Stop()

	s.runRetentionOnce()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runRetentionOnce()
		}
	}
}

func (s *AuditLogService) runRetentionOnce() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancel()

	days := 0
	if s.settingService != nil {
		days = s.settingService.GetAuditLogRetentionDays(ctx)
	}
	if days <= 0 {
		return // 0 或负值表示永久保留，仅支持手动清空
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	for {
		deleted, err := s.repo.DeleteBefore(ctx, cutoff, auditRetentionBatchSize)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "time=%s level=WARN msg=\"audit log retention cleanup failed\" err=%v\n",
				time.Now().Format(time.RFC3339Nano), err)
			return
		}
		if deleted == 0 {
			return
		}
	}
}
