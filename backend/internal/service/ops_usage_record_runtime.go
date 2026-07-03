package service

import "context"

// GetUsageRecordRuntimeStats returns process-global usage record runtime health.
func (s *OpsService) GetUsageRecordRuntimeStats(ctx context.Context) (UsageRecordRuntimeStats, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return UsageRecordRuntimeStats{}, err
	}
	return GatewayUsageRecordRuntimeStats(s.usageRecordWorkerPool), nil
}
