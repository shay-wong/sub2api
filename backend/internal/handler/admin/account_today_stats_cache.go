package admin

import (
	"strconv"
	"strings"
	"time"
)

var accountTodayStatsBatchCache = newSnapshotCache(30 * time.Second)

func buildAccountTodayStatsBatchCacheKey(projectID int64, accountIDs []int64) string {
	if len(accountIDs) == 0 {
		if projectID > 0 {
			return "accounts_today_stats:p" + strconv.FormatInt(projectID, 10) + ":empty"
		}
		return "accounts_today_stats:p0:empty"
	}
	var b strings.Builder
	b.Grow(len(accountIDs) * 6)
	_, _ = b.WriteString("accounts_today_stats:p")
	_, _ = b.WriteString(strconv.FormatInt(projectID, 10))
	_ = b.WriteByte(':')
	for i, id := range accountIDs {
		if i > 0 {
			_ = b.WriteByte(',')
		}
		_, _ = b.WriteString(strconv.FormatInt(id, 10))
	}
	return b.String()
}
