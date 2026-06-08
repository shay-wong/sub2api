//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release *GitHubRelease
	repo    string
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.repo = repo
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func TestUpdateServiceChecksForkRepository(t *testing.T) {
	github := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v0.1.134-fork.2",
			Name:    "v0.1.134-fork.2",
		},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, github, "0.1.134-fork.1", "release")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, "shay-wong/sub2api", github.repo)
	require.Equal(t, "0.1.134-fork.2", info.LatestVersion)
	require.True(t, info.HasUpdate)
}

func TestCompareVersionsSupportsForkReleaseSuffix(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{
			name:    "newer fork suffix is update",
			current: "0.1.134-fork.1",
			latest:  "0.1.134-fork.2",
			want:    -1,
		},
		{
			name:    "older fork suffix is not update",
			current: "0.1.134-fork.2",
			latest:  "0.1.134-fork.1",
			want:    1,
		},
		{
			name:    "higher upstream base outranks fork suffix",
			current: "0.1.134-fork.9",
			latest:  "0.1.135-fork.1",
			want:    -1,
		},
		{
			name:    "fork release outranks same base plain release",
			current: "0.1.134-fork.9",
			latest:  "0.1.134",
			want:    1,
		},
		{
			name:    "same base fork release outranks plain release",
			current: "0.1.134",
			latest:  "0.1.134-fork.1",
			want:    -1,
		},
		{
			name:    "current fork does not update to same base plain release",
			current: "0.1.135-fork.1",
			latest:  "0.1.135",
			want:    1,
		},
		{
			name:    "higher fork release outranks same base plain release",
			current: "0.1.135-fork.2",
			latest:  "0.1.135",
			want:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, compareVersions(tt.current, tt.latest))
		})
	}
}
