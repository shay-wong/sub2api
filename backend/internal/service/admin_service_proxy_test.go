//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateProxyDefaultsExpiryWarnDays(t *testing.T) {
	repo := &proxyRepoStubForAdminProxy{}
	svc := &adminServiceImpl{proxyRepo: repo}

	proxy, err := svc.CreateProxy(context.Background(), &CreateProxyInput{
		Name:     "proxy",
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     8080,
	})

	require.NoError(t, err)
	require.Equal(t, 7, proxy.ExpiryWarnDays)
	require.Equal(t, 7, repo.created.ExpiryWarnDays)
}

func TestAdminService_UpdateProxyPartialStatusPreservesOptionalFields(t *testing.T) {
	expiresAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	backupProxyID := int64(42)
	repo := &proxyRepoStubForAdminProxy{
		proxy: &Proxy{
			ID:             1,
			Name:           "proxy",
			Protocol:       "http",
			Host:           "127.0.0.1",
			Port:           8080,
			Status:         StatusActive,
			ExpiresAt:      &expiresAt,
			FallbackMode:   FallbackModeProxy,
			BackupProxyID:  &backupProxyID,
			ExpiryWarnDays: 9,
		},
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	proxy, err := svc.UpdateProxy(context.Background(), 1, &UpdateProxyInput{
		Status: StatusDisabled,
	})

	require.NoError(t, err)
	require.Equal(t, StatusDisabled, proxy.Status)
	require.Equal(t, &expiresAt, proxy.ExpiresAt)
	require.Equal(t, FallbackModeProxy, proxy.FallbackMode)
	require.Equal(t, &backupProxyID, proxy.BackupProxyID)
	require.Equal(t, 9, proxy.ExpiryWarnDays)
	require.Equal(t, repo.updated, proxy)
}

func TestAdminService_UpdateProxyExplicitlyClearsOrSetsOptionalFields(t *testing.T) {
	expiresAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	backupProxyID := int64(42)
	warnDays := 0
	repo := &proxyRepoStubForAdminProxy{
		proxy: &Proxy{
			ID:             1,
			Name:           "proxy",
			Protocol:       "http",
			Host:           "127.0.0.1",
			Port:           8080,
			Status:         StatusActive,
			ExpiresAt:      &expiresAt,
			FallbackMode:   FallbackModeProxy,
			BackupProxyID:  &backupProxyID,
			ExpiryWarnDays: 9,
		},
	}
	svc := &adminServiceImpl{proxyRepo: repo}

	proxy, err := svc.UpdateProxy(context.Background(), 1, &UpdateProxyInput{
		ExpiresAtProvided:      true,
		FallbackModeProvided:   true,
		FallbackMode:           "",
		BackupProxyIDProvided:  true,
		BackupProxyID:          nil,
		ExpiryWarnDaysProvided: true,
		ExpiryWarnDays:         &warnDays,
	})

	require.NoError(t, err)
	require.Nil(t, proxy.ExpiresAt)
	require.Equal(t, FallbackModeNone, proxy.FallbackMode)
	require.Nil(t, proxy.BackupProxyID)
	require.Equal(t, 0, proxy.ExpiryWarnDays)
}

type proxyRepoStubForAdminProxy struct {
	proxyRepoStub

	proxy   *Proxy
	created *Proxy
	updated *Proxy
}

func (s *proxyRepoStubForAdminProxy) Create(_ context.Context, proxy *Proxy) error {
	s.created = proxy
	return nil
}

func (s *proxyRepoStubForAdminProxy) GetByID(_ context.Context, _ int64) (*Proxy, error) {
	if s.proxy == nil {
		return nil, ErrProxyNotFound
	}
	copy := *s.proxy
	return &copy, nil
}

func (s *proxyRepoStubForAdminProxy) Update(_ context.Context, proxy *Proxy) error {
	s.updated = proxy
	s.proxy = proxy
	return nil
}
