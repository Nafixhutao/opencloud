package model

// Git and build job kinds.
const (
	JobCloneGitSource     = "clone_git_source"
	JobBuildSource        = "build_source"
	JobDeployPreview      = "deploy_preview"
	JobDestroyPreview     = "destroy_preview"
)

const (
	MaxCloneAttempts   = 3
	MaxBuildAttempts   = 3
	MaxDeployAttempts  = 3
)

// CloneGitSourcePayload is the job payload for cloning a git source.
type CloneGitSourcePayload struct {
	ServiceID string `json:"service_id"`
}
