package provisioner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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
	if err := validateCloneSpec(spec); err != nil {
		return "", err
	}
	args := []string{"clone", "--depth", "1"}
	if spec.Branch != "" {
		args = append(args, "--branch", spec.Branch)
	}
	args = append(args, spec.URL, spec.TargetDir)

	cmd := exec.CommandContext(ctx, "git", args...)
	// Hardening: no interactive credential prompts in a worker, and only
	// http(s) may ever be used as a transport (blocks ext::/fd:: helpers).
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/true",
		"GIT_ALLOW_PROTOCOL=https,http",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}

	if spec.CommitSHA != "" && spec.Branch == "" {
		cmd2 := exec.CommandContext(ctx, "git", "-C", spec.TargetDir, "checkout", spec.CommitSHA)
		cmd2.Env = cmd.Env
		out2, err := cmd2.CombinedOutput()
		if err != nil {
			return strings.TrimSpace(string(out2)), err
		}
	}

	return "", nil
}

// validateCloneSpec rejects repository values that git would interpret as
// options or exotic transports. A URL/branch starting with "-" is parsed by
// git as a flag (e.g. --upload-pack), and schemes like ext:: execute helper
// commands — both are remote-code-execution vectors for user-supplied repos.
func validateCloneSpec(spec GitCloneSpec) error {
	if !strings.HasPrefix(spec.URL, "https://") && !strings.HasPrefix(spec.URL, "http://") {
		return fmt.Errorf("git repository URL must use https:// or http://: %q", spec.URL)
	}
	if strings.HasPrefix(spec.Branch, "-") {
		return fmt.Errorf("git branch name is not allowed: %q", spec.Branch)
	}
	if spec.CommitSHA != "" && !commitSHAPattern.MatchString(spec.CommitSHA) {
		return fmt.Errorf("git commit sha is malformed: %q", spec.CommitSHA)
	}
	return nil
}

var commitSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// FakeGitProvisioner is a no-op git provisioner for testing.
type FakeGitProvisioner struct{}

// Clone is a no-op.
func (f *FakeGitProvisioner) Clone(_ context.Context, _ GitCloneSpec) (string, error) {
	return "", nil
}

var _ GitProvisioner = (*LocalGitProvisioner)(nil)
var _ GitProvisioner = (*FakeGitProvisioner)(nil)
