package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userAttributeDefinitionRepoStub struct {
	listCalls int
}

func (r *userAttributeDefinitionRepoStub) Create(context.Context, *service.UserAttributeDefinition) error {
	return nil
}

func (r *userAttributeDefinitionRepoStub) GetByID(context.Context, int64) (*service.UserAttributeDefinition, error) {
	return nil, service.ErrAttributeDefinitionNotFound
}

func (r *userAttributeDefinitionRepoStub) GetByKey(context.Context, string) (*service.UserAttributeDefinition, error) {
	return nil, service.ErrAttributeDefinitionNotFound
}

func (r *userAttributeDefinitionRepoStub) Update(context.Context, *service.UserAttributeDefinition) error {
	return nil
}

func (r *userAttributeDefinitionRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (r *userAttributeDefinitionRepoStub) List(context.Context, bool) ([]service.UserAttributeDefinition, error) {
	r.listCalls++
	return []service.UserAttributeDefinition{
		{
			ID:        1,
			Key:       "tier",
			Name:      "Tier",
			Type:      service.AttributeTypeText,
			Enabled:   true,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}, nil
}

func (r *userAttributeDefinitionRepoStub) UpdateDisplayOrders(context.Context, map[int64]int) error {
	return nil
}

func (r *userAttributeDefinitionRepoStub) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}

type userAttributeValueRepoStub struct {
	getByUserIDCalls  int
	getByUserIDsCalls int
	upsertBatchCalls  int
}

func (r *userAttributeValueRepoStub) GetByUserID(context.Context, int64) ([]service.UserAttributeValue, error) {
	r.getByUserIDCalls++
	return []service.UserAttributeValue{}, nil
}

func (r *userAttributeValueRepoStub) GetByUserIDs(context.Context, []int64) ([]service.UserAttributeValue, error) {
	r.getByUserIDsCalls++
	return []service.UserAttributeValue{}, nil
}

func (r *userAttributeValueRepoStub) UpsertBatch(context.Context, int64, []service.UpdateUserAttributeInput) error {
	r.upsertBatchCalls++
	return nil
}

func (r *userAttributeValueRepoStub) DeleteByAttributeID(context.Context, int64) error {
	return nil
}

func (r *userAttributeValueRepoStub) DeleteByUserID(context.Context, int64) error {
	return nil
}

func TestUserAttributeHandlerRejectsOutOfScopeUserBeforeReadingAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valueRepo := &userAttributeValueRepoStub{}
	handler := NewUserAttributeHandler(
		service.NewUserAttributeService(&userAttributeDefinitionRepoStub{}, valueRepo),
		&stubAdminService{getUserErr: service.ErrUserNotFound},
	)
	router := gin.New()
	router.GET("/admin/users/:id/attributes", handler.GetUserAttributes)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/users/42/attributes", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "USER_NOT_FOUND")
	require.Zero(t, valueRepo.getByUserIDCalls)
}

func TestUserAttributeHandlerRejectsOutOfScopeUserBeforeUpdatingAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defRepo := &userAttributeDefinitionRepoStub{}
	valueRepo := &userAttributeValueRepoStub{}
	handler := NewUserAttributeHandler(
		service.NewUserAttributeService(defRepo, valueRepo),
		&stubAdminService{getUserErr: service.ErrUserNotFound},
	)
	router := gin.New()
	router.PUT("/admin/users/:id/attributes", handler.UpdateUserAttributes)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/users/42/attributes", bytes.NewBufferString(`{"values":{"1":"gold"}}`))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "USER_NOT_FOUND")
	require.Zero(t, defRepo.listCalls)
	require.Zero(t, valueRepo.upsertBatchCalls)
	require.Zero(t, valueRepo.getByUserIDCalls)
}

func TestUserAttributeHandlerRejectsOutOfScopeUserBeforeBatchReadingAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valueRepo := &userAttributeValueRepoStub{}
	handler := NewUserAttributeHandler(
		service.NewUserAttributeService(&userAttributeDefinitionRepoStub{}, valueRepo),
		&stubAdminService{getUserErr: service.ErrUserNotFound},
	)
	router := gin.New()
	router.POST("/admin/user-attributes/batch", handler.GetBatchUserAttributes)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/user-attributes/batch", bytes.NewBufferString(`{"user_ids":[42,42,51]}`))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "USER_NOT_FOUND")
	require.Zero(t, valueRepo.getByUserIDsCalls)
}
