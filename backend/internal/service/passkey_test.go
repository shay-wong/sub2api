package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

func TestNormalizePasskeyName(t *testing.T) {
	require.Equal(t, defaultPasskeyName, normalizePasskeyName("   "))
	require.Equal(t, "Laptop", normalizePasskeyName("  Laptop  "))

	longName := strings.Repeat("密", maxPasskeyNameLength+10)
	require.Len(t, []rune(normalizePasskeyName(longName)), maxPasskeyNameLength)
}

func TestPasskeySummaryReportsCurrentBackupState(t *testing.T) {
	record := &PasskeyCredentialRecord{
		Credential: webauthn.Credential{
			Flags: webauthn.CredentialFlags{BackupEligible: true},
		},
	}
	require.False(t, passkeySummary(record).Backup)

	record.Credential.Flags.BackupState = true
	require.True(t, passkeySummary(record).Backup)
}

// 桩仅实现测试所需方法；未桩方法调用即 panic（嵌入 nil 接口）。
type passkeyPwUserRepoStub struct {
	UserRepository
	user *User
}

func (s *passkeyPwUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type passkeyPwRepoStub struct {
	PasskeyRepository
	handleCalled bool
	deleteCalled bool
}

func (s *passkeyPwRepoStub) EnsureUserHandle(_ context.Context, _ int64, candidate []byte) ([]byte, error) {
	s.handleCalled = true
	return candidate, nil
}

func (s *passkeyPwRepoStub) ListByUserID(context.Context, int64) ([]PasskeyCredentialRecord, error) {
	return nil, nil
}

func (s *passkeyPwRepoStub) Delete(context.Context, int64, int64) error {
	s.deleteCalled = true
	return nil
}

type passkeyPwSessionStoreStub struct {
	PasskeySessionStore
}

func (s *passkeyPwSessionStoreStub) Store(context.Context, *PasskeySession, time.Duration) (string, error) {
	return "session-token", nil
}

type passkeyIdentitySettingRepo struct {
	SettingRepository
	emailVerifyEnabled bool
}

func (s *passkeyIdentitySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if key == SettingKeyEmailVerifyEnabled && s.emailVerifyEnabled {
		return "true", nil
	}
	return "", errors.New("setting not found")
}

type passkeyEmailCacheStub struct {
	EmailCache
	data *VerificationCodeData
}

func (s *passkeyEmailCacheStub) GetVerificationCode(context.Context, string) (*VerificationCodeData, error) {
	return s.data, nil
}

func (*passkeyEmailCacheStub) DeleteVerificationCode(context.Context, string) error {
	return nil
}

func newPasskeyPwService(t *testing.T, user *User) (*PasskeyService, *passkeyPwRepoStub) {
	return newPasskeyPwServiceWithEmailVerification(t, user, false)
}

func newPasskeyPwServiceWithEmailVerification(
	t *testing.T,
	user *User,
	emailVerifyEnabled bool,
) (*PasskeyService, *passkeyPwRepoStub) {
	t.Helper()
	repo := &passkeyPwRepoStub{}
	userRepo := &passkeyPwUserRepoStub{user: user}
	identity := NewTotpService(
		userRepo,
		nil,
		nil,
		NewSettingService(&passkeyIdentitySettingRepo{emailVerifyEnabled: emailVerifyEnabled}, nil),
		nil,
		nil,
	)
	svc, err := NewPasskeyService(&config.Config{WebAuthn: config.WebAuthnConfig{
		Enabled:       true,
		RPDisplayName: "Sub2API",
		RPID:          "sub2api.example.com",
		RPOrigins:     []string{"https://sub2api.example.com"},
	}}, repo, &passkeyPwSessionStoreStub{}, userRepo, identity)
	require.NoError(t, err)
	return svc, repo
}

// 有本地密码的账号注册与吊销必须验证账号密码：被窃会话不得静默添加/移除凭据。
func TestPasskeyEnrollmentAndRevocationRequireAccountPassword(t *testing.T) {
	user := &User{ID: 7, Email: "user@example.com", Status: StatusActive}
	require.NoError(t, user.SetPassword("correct-password"))
	svc, repo := newPasskeyPwServiceWithEmailVerification(t, user, true)

	method, err := svc.GetVerificationMethod(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "password", method.Method)
	require.Error(t, svc.SendVerifyCode(context.Background(), user.ID))

	_, _, err = svc.BeginRegistration(context.Background(), user.ID, "", "")
	require.ErrorIs(t, err, ErrPasswordRequired)
	_, _, err = svc.BeginRegistration(context.Background(), user.ID, "", "wrong-password")
	require.ErrorIs(t, err, ErrPasswordIncorrect)
	require.False(t, repo.handleCalled)

	creation, token, err := svc.BeginRegistration(context.Background(), user.ID, "", "correct-password")
	require.NoError(t, err)
	require.NotNil(t, creation)
	require.Equal(t, "session-token", token)
	require.True(t, repo.handleCalled)

	err = svc.Delete(context.Background(), user.ID, 1, "", "")
	require.ErrorIs(t, err, ErrPasswordRequired)
	err = svc.Delete(context.Background(), user.ID, 1, "", "wrong-password")
	require.ErrorIs(t, err, ErrPasswordIncorrect)
	require.False(t, repo.deleteCalled)

	require.NoError(t, svc.Delete(context.Background(), user.ID, 1, "", "correct-password"))
	require.True(t, repo.deleteCalled)
}

func TestPasskeyOAuthOnlyUserRequiresEmailCode(t *testing.T) {
	user := &User{
		ID:                   8,
		Email:                "oauth@example.com",
		Role:                 RoleUser,
		Status:               StatusActive,
		PasswordAuthDisabled: true,
		PasswordAuthResolved: true,
	}
	svc, repo := newPasskeyPwService(t, user)
	method, err := svc.GetVerificationMethod(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "email", method.Method)

	_, _, err = svc.BeginRegistration(context.Background(), user.ID, "", "unknown-random-password")
	require.ErrorIs(t, err, ErrVerifyCodeRequired)
	require.False(t, repo.handleCalled)

	err = svc.Delete(context.Background(), user.ID, 1, "", "unknown-random-password")
	require.ErrorIs(t, err, ErrVerifyCodeRequired)
	require.False(t, repo.deleteCalled)

	svc.identity.emailService = &EmailService{cache: &passkeyEmailCacheStub{data: &VerificationCodeData{
		Code:      "123456",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	}}}
	creation, token, err := svc.BeginRegistration(context.Background(), user.ID, "123456", "")
	require.NoError(t, err)
	require.NotNil(t, creation)
	require.Equal(t, "session-token", token)
	require.True(t, repo.handleCalled)

	svc.identity.emailService = &EmailService{cache: &passkeyEmailCacheStub{data: &VerificationCodeData{
		Code:      "654321",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	}}}
	require.NoError(t, svc.Delete(context.Background(), user.ID, 1, "654321", ""))
	require.True(t, repo.deleteCalled)
}

func TestPasskeyOAuthOnlyAdminRemainsPasswordOnly(t *testing.T) {
	admin := &User{
		ID:                   9,
		Email:                "admin@example.com",
		Role:                 RoleAdmin,
		Status:               StatusActive,
		PasswordAuthDisabled: true,
		PasswordAuthResolved: true,
	}
	svc, repo := newPasskeyPwServiceWithEmailVerification(t, admin, true)

	method, err := svc.GetVerificationMethod(context.Background(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, "password", method.Method)
	require.Error(t, svc.SendVerifyCode(context.Background(), admin.ID))

	_, _, err = svc.BeginRegistration(context.Background(), admin.ID, "123456", "")
	require.ErrorIs(t, err, ErrPasswordRequired)
	require.False(t, repo.handleCalled)

	err = svc.Delete(context.Background(), admin.ID, 1, "123456", "")
	require.ErrorIs(t, err, ErrPasswordRequired)
	require.False(t, repo.deleteCalled)
}
