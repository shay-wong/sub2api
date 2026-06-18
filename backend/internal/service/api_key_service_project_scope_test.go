//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type apiKeyCreateRepoStub struct {
	APIKeyRepository
	created []*APIKey
}

func (s *apiKeyCreateRepoStub) Create(_ context.Context, key *APIKey) error {
	clone := *key
	s.created = append(s.created, &clone)
	return nil
}

func (s *apiKeyCreateRepoStub) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}

type apiKeyCreateUserRepoStub struct {
	UserRepository
	user *User
}

func (s *apiKeyCreateUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	clone := *s.user
	return &clone, nil
}

type apiKeyCreateGroupRepoStub struct {
	GroupRepository
	group *Group
}

func (s *apiKeyCreateGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	clone := *s.group
	return &clone, nil
}

func TestAPIKeyServiceCreateAllowsProfileVisibleGroupFromDifferentHomeProject(t *testing.T) {
	groupID := int64(9)
	apiKeyRepo := &apiKeyCreateRepoStub{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &apiKeyCreateUserRepoStub{user: &User{
			ID:            42,
			Status:        StatusActive,
			AllowedGroups: []int64{groupID},
		}},
		groupRepo: &apiKeyCreateGroupRepoStub{group: &Group{
			ID:          groupID,
			ProjectID:   200,
			Status:      StatusActive,
			IsExclusive: true,
		}},
	}

	customKey := "custom-project-key-0001"
	key, err := svc.Create(WithProjectID(context.Background(), 100), 42, CreateAPIKeyRequest{
		Name:      "cross-project-group",
		GroupID:   &groupID,
		CustomKey: &customKey,
	})
	require.NoError(t, err)
	require.NotNil(t, key)
	require.Len(t, apiKeyRepo.created, 1)
	require.NotNil(t, apiKeyRepo.created[0].GroupID)
	require.Equal(t, groupID, *apiKeyRepo.created[0].GroupID)
}
