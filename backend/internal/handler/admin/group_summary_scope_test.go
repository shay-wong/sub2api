package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupSummariesUseDirectGroupBindings(t *testing.T) {
	scope := &adminAccessScope{GroupIDs: []int64{2}, groupSet: int64Set([]int64{2})}

	usage := filterGroupUsageSummaries(scope, []usagestats.GroupUsageSummary{{GroupID: 1}, {GroupID: 2}})
	capacity := filterGroupCapacitySummaries(scope, []service.GroupCapacitySummary{{GroupID: 1}, {GroupID: 2}})

	require.Equal(t, []usagestats.GroupUsageSummary{{GroupID: 2}}, usage)
	require.Equal(t, []service.GroupCapacitySummary{{GroupID: 2}}, capacity)
}
