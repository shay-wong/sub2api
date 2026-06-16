package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpsAlertEventsWhere_AppliesOperatorGroupScope(t *testing.T) {
	filter := &service.OpsAlertEventFilter{
		GroupIDs: []int64{10, 20, -1},
	}

	where, args := buildOpsAlertEventsWhere(filter)
	if !strings.Contains(where, "(dimensions->>'group_id') = ANY($") {
		t.Fatalf("where should include group scope ANY condition: %s", where)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
}

func TestBuildOpsAlertEventsWhere_EmptyOperatorScopeReturnsNoRows(t *testing.T) {
	filter := &service.OpsAlertEventFilter{GroupScopeEmpty: true}

	where, _ := buildOpsAlertEventsWhere(filter)
	if !strings.Contains(where, "1 = 0") {
		t.Fatalf("where should include impossible predicate for empty scope: %s", where)
	}
}

func TestListRequestDetailsScopeFieldsAreRecognizedByFilter(t *testing.T) {
	filter := &service.OpsRequestDetailFilter{
		StartTime: timePtr(time.Now().Add(-time.Hour)),
		EndTime:   timePtr(time.Now()),
		GroupIDs:  []int64{10, 20},
	}

	page, pageSize, start, end := filter.Normalize()
	if page != 1 || pageSize != 50 {
		t.Fatalf("page defaults = %d/%d, want 1/50", page, pageSize)
	}
	if start.IsZero() || end.IsZero() {
		t.Fatalf("time window should be normalized")
	}
	if len(filter.GroupIDs) != 2 {
		t.Fatalf("group scope should remain attached to filter")
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
