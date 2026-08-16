package provisioner

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCloneSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    GitCloneSpec
		wantErr bool
	}{
		{name: "https url", spec: GitCloneSpec{URL: "https://github.com/acme/app.git", Branch: "main"}},
		{name: "http url for dev mirrors", spec: GitCloneSpec{URL: "http://gitea.local/acme/app.git"}},
		{name: "ssh url rejected", spec: GitCloneSpec{URL: "git@github.com:acme/app.git"}, wantErr: true},
		{name: "ext transport rejected", spec: GitCloneSpec{URL: "ext::sh -c id"}, wantErr: true},
		{name: "fd transport rejected", spec: GitCloneSpec{URL: "fd::17"}, wantErr: true},
		{name: "leading dash url rejected", spec: GitCloneSpec{URL: "--upload-pack=touch /tmp/pwn"}, wantErr: true},
		{name: "file path rejected", spec: GitCloneSpec{URL: "/etc/passwd"}, wantErr: true},
		{name: "local path with colon rejected", spec: GitCloneSpec{URL: "../../repo"}, wantErr: true},
		{name: "branch leading dash rejected", spec: GitCloneSpec{URL: "https://github.com/acme/app.git", Branch: "--upload-pack=sh"}, wantErr: true},
		{name: "normal branch", spec: GitCloneSpec{URL: "https://github.com/acme/app.git", Branch: "feature/x-1"}},
		{name: "valid commit sha", spec: GitCloneSpec{URL: "https://github.com/acme/app.git", CommitSHA: "9f86d081884c7d65"}},
		{name: "malformed commit sha rejected", spec: GitCloneSpec{URL: "https://github.com/acme/app.git", CommitSHA: "touch$IFS/tmp"}, wantErr: true},
		{name: "empty url rejected", spec: GitCloneSpec{URL: ""}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCloneSpec(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCloneRejectsUnsafeSpecBeforeExec(t *testing.T) {
	g := NewLocalGitProvisioner()
	out, err := g.Clone(t.Context(), GitCloneSpec{
		URL:       "ext::sh -c id",
		TargetDir: t.TempDir(),
	})
	require.Error(t, err)
	require.Equal(t, "", out)
	require.Contains(t, err.Error(), "https://")
}

func TestCloneRejectsBranchInjection(t *testing.T) {
	g := NewLocalGitProvisioner()
	_, err := g.Clone(t.Context(), GitCloneSpec{
		URL:       "https://github.com/acme/app.git",
		Branch:    "--upload-pack=sh",
		TargetDir: t.TempDir(),
	})
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "git branch name is not allowed"))
}
