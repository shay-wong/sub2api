package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAuditLogRepositoryBatchInsertSkipsIDsAtOrBelowLastClear(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	const clearedID = int64(10)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("LOCK TABLE audit_logs IN ROW EXCLUSIVE MODE")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE((SELECT clear_id FROM audit_log_state WHERE singleton = TRUE), 0)")).
		WillReturnRows(sqlmock.NewRows([]string{"clear_id"}).AddRow(clearedID))
	copyStatement := mock.ExpectPrepare(`COPY .*audit_logs`)
	copyStatement.ExpectExec().WithArgs(
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), "after-clear", sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(11),
	).WillReturnResult(sqlmock.NewResult(0, 1))
	copyStatement.ExpectExec().WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	repo := NewAuditLogRepository(db)
	inserted, err := repo.BatchInsert(context.Background(), []*service.AuditLog{
		{ID: 9, Action: "before-clear"},
		{ID: 11, Action: "after-clear"},
	})
	if err != nil {
		t.Fatalf("BatchInsert() error = %v", err)
	}
	if inserted != 1 {
		t.Fatalf("BatchInsert() inserted = %d, want 1", inserted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestAuditLogRepositoryClearAllRollsBackWhenTraceInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("LOCK TABLE audit_logs IN ACCESS EXCLUSIVE MODE")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT nextval('audit_logs_id_seq')")).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(42)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM audit_logs")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))
	mock.ExpectExec(regexp.QuoteMeta("TRUNCATE TABLE audit_logs")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO audit_log_state").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	repo := NewAuditLogRepository(db)
	deleted, err := repo.ClearAll(context.Background(), &service.AuditLog{})
	if err == nil {
		t.Fatal("ClearAll() error = nil, want insert failure")
	}
	if deleted != 0 {
		t.Fatalf("ClearAll() deleted = %d, want 0 after rollback", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
