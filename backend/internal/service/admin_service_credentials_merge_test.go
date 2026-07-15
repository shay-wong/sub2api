//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type updateAccountCredsRepoStub struct {
	mockAccountRepoForGemini
	account     *Account
	updateCalls int
}

func (r *updateAccountCredsRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	return r.account, nil
}

func (r *updateAccountCredsRepoStub) Update(ctx context.Context, account *Account) error {
	r.updateCalls++
	r.account = account
	return nil
}

func TestUpdateAccount_PreservesSensitiveCredsWhenIncomingOmits(t *testing.T) {
	accountID := int64(202)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Credentials: map[string]any{
				"refresh_token": "rt-existing",
				"access_token":  "at-existing",
				"id_token":      "id-existing",
				"base_url":      "https://old.example.com",
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	// 模拟前端编辑：仅修改 base_url，没有传 token（脱敏后前端 spread 拿不到敏感键）
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{
			"base_url": "https://new.example.com",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)

	// 敏感键应保留
	require.Equal(t, "rt-existing", repo.account.Credentials["refresh_token"])
	require.Equal(t, "at-existing", repo.account.Credentials["access_token"])
	require.Equal(t, "id-existing", repo.account.Credentials["id_token"])
	// 非敏感键被替换
	require.Equal(t, "https://new.example.com", repo.account.Credentials["base_url"])
}

func TestUpdateAccount_ExplicitNewTokenOverwrites(t *testing.T) {
	accountID := int64(203)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Credentials: map[string]any{
				"refresh_token": "rt-old",
				"api_key":       "sk-old",
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{
			"refresh_token": "rt-new",
			// api_key 没传 → 应保留旧值
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.Equal(t, "rt-new", repo.account.Credentials["refresh_token"])
	require.Equal(t, "sk-old", repo.account.Credentials["api_key"])
}

func TestUpdateAccount_ReplaceCredentialsDoesNotRestoreSensitiveValues(t *testing.T) {
	accountID := int64(208)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Credentials: map[string]any{
				"access_token":  "stale-access",
				"refresh_token": "stale-refresh",
				"id_token":      "stale-id",
				"plan_type":     "pro",
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{
			"auth_mode":        OpenAIAuthModeAgentIdentity,
			"agent_runtime_id": "runtime-replacement",
			"model_mapping":    map[string]any{"gpt-old": "gpt-new"},
		},
		ReplaceCredentials: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, updated.Credentials, repo.account.Credentials)
	require.Equal(t, OpenAIAuthModeAgentIdentity, repo.account.Credentials["auth_mode"])
	require.NotContains(t, repo.account.Credentials, "access_token")
	require.NotContains(t, repo.account.Credentials, "refresh_token")
	require.NotContains(t, repo.account.Credentials, "id_token")
	require.NotContains(t, repo.account.Credentials, "plan_type")
	require.Contains(t, repo.account.Credentials, "model_mapping")
}

func TestUpdateAccount_EmptyCredentialsSkipsUpdate(t *testing.T) {
	accountID := int64(204)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Credentials: map[string]any{
				"refresh_token": "rt-existing",
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials: map[string]any{}, // len == 0 → 闸门跳过
		Name:        "renamed",
	})
	require.NoError(t, err)

	require.Equal(t, "rt-existing", repo.account.Credentials["refresh_token"], "空 credentials 不应触碰已有 token")
	require.Equal(t, "renamed", repo.account.Name)
}

func TestUpdateAccount_ClearPlanTypePreservesSensitiveCredentials(t *testing.T) {
	accountID := int64(207)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Credentials: map[string]any{
				"access_token": "at-existing",
				"plan_type":    "pro",
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Credentials:   map[string]any{},
		ClearPlanType: true,
	})
	require.NoError(t, err)

	require.Equal(t, "at-existing", repo.account.Credentials["access_token"])
	require.NotContains(t, repo.account.Credentials, "plan_type")
}

func TestUpdateAccount_WithGroupScopePreservesOutOfScopeGroups(t *testing.T) {
	accountID := int64(205)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			GroupIDs: []int64{10, 30},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo: &groupRepoStubForAdmin{
			getByID: &Group{ID: 10, Name: "visible"},
		},
	}

	nextVisibleGroups := []int64{10}
	_, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Name:          "renamed",
		GroupIDs:      &nextVisibleGroups,
		GroupScopeIDs: []int64{10},
	})

	require.NoError(t, err)
	require.Equal(t, []int64{10, 30}, repo.boundGroupsByID[accountID])
}

func TestUpdateAccount_WithGroupScopeCanRemoveVisibleGroupWithoutRemovingHiddenGroup(t *testing.T) {
	accountID := int64(206)
	repo := &updateAccountCredsRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			GroupIDs: []int64{10, 30},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo: &groupRepoStubForAdmin{
			getByID: &Group{ID: 30, Name: "hidden"},
		},
	}

	nextVisibleGroups := []int64{}
	_, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		GroupIDs:      &nextVisibleGroups,
		GroupScopeIDs: []int64{10},
	})

	require.NoError(t, err)
	require.Equal(t, []int64{30}, repo.boundGroupsByID[accountID])
}
