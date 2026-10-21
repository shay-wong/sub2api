package admin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportCodexSessionsKeepsAgentIdentityWorkspaceMembersSeparate(t *testing.T) {
	svc := newCodexImportMemoryAdminService(nil)
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entries := []codexImportEntry{
		{Index: 1, Value: buildAgentIdentityImportValue(t, "runtime-a", "team-shared", "user-a", "task-a")},
		{Index: 2, Value: buildAgentIdentityImportValue(t, "runtime-b", "team-shared", "user-b", "task-b")},
	}

	result, err := handler.importCodexSessions(context.Background(), CodexSessionImportRequest{SkipDefaultGroupBind: boolPtr(true)}, entries)
	require.NoError(t, err)
	require.Equal(t, 2, result.Created)
	require.Zero(t, result.Updated)
	require.Zero(t, result.Skipped)
	require.Zero(t, result.Failed)
	require.Len(t, svc.createdAccounts, 2)
	require.Equal(t, "user-a", svc.createdAccounts[0].Credentials["chatgpt_user_id"])
	require.Equal(t, "user-b", svc.createdAccounts[1].Credentials["chatgpt_user_id"])
}
