package provisioner

import (
	"context"
	"os/exec"
	"strings"
)

// GitProvisioner handles git repository operations.
type GitProvisioner interface {
	Clone(ctx context.Context, spec GitCloneSpec) (string, error)
}

// GitCloneSpec defines a git clone operation.
type GitCloneSpec struct {
	URL       string
	Branch    string
	CommitSHA string
	TargetDir string
}

// LocalGitProvisioner clones git repositories using the system git binary.
type LocalGitProvisioner struct{}

// NewLocalGitProvisioner creates a local git provisioner.
func NewLocalGitProvisioner() *LocalGitProvisioner {
	return &LocalGitProvisioner{}
}

// Clone clones a git repository to the target directory.
func (g *LocalGitProvisioner) Clone(ctx context.Context, spec GitCloneSpec) (string, error) {
	args := []string{"clone", "--depth", "1"}
	if spec.Branch != "" {
		args = append(args, "--branch", spec.Branch)
	}
	args = append(args, spec.URL, spec.TargetDir)

	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}

	if spec.CommitSHA != "" && spec.Branch == "" {
		cmd2 := exec.CommandContext(ctx, "git", "-C", spec.TargetDir, "checkout", spec.CommitSHA)
		out2, err := cmd2.CombinedOutput()
		if err != nil {
			return strings.TrimSpace(string(out2)), err
		}
	}

	return "", nil
}

// FakeGitProvisioner is a no-op git provisioner for testing.
type FakeGitProvisioner struct{}

// Clone is a no-op.
func (f *FakeGitProvisioner) Clone(_ context.Context, _ GitCloneSpec) (string, error) {
	return "", nil
}

var _ GitProvisioner = (*LocalGitProvisioner)(nil)
var _ GitProvisioner = (*FakeGitProvisioner)(nil)
