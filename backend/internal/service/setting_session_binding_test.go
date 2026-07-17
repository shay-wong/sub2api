//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type sessionBindingSettingRepoStub struct {
	SettingRepository
	value string
	err   error
}

func (s *sessionBindingSettingRepoStub) GetValue(context.Context, string) (string, error) {
	return s.value, s.err
}

func TestSessionBindingDefaultsDisabledWhenSettingIsMissing(t *testing.T) {
	svc := NewSettingService(&sessionBindingSettingRepoStub{err: ErrSettingNotFound}, &config.Config{})
	require.False(t, svc.IsSessionBindingEnabled(context.Background()))
}

func TestSessionBindingCanBeExplicitlyEnabled(t *testing.T) {
	svc := NewSettingService(&sessionBindingSettingRepoStub{value: "true"}, &config.Config{})
	require.True(t, svc.IsSessionBindingEnabled(context.Background()))
}

func TestSessionBindingEmptySettingStaysDisabled(t *testing.T) {
	svc := NewSettingService(&sessionBindingSettingRepoStub{}, &config.Config{})
	require.False(t, svc.IsSessionBindingEnabled(context.Background()))
}
