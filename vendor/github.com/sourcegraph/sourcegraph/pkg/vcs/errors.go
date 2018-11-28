package vcs

import "github.com/sourcegraph/sourcegraph/pkg/api"

// RepoNotExistError is an error that reports a repository doesn't exist.
type RepoNotExistError struct {
	Repo api.RepoName

	// CloneInProgress reports whether the repository is in process of being cloned.
	CloneInProgress bool

	// CloneProgress is a progress message from the running clone command.
	CloneProgress string
}

func (RepoNotExistError) NotFound() bool { return true }

func (e *RepoNotExistError) Error() string {
	if e.CloneInProgress {
		return "repository does not exist (clone in progress): " + string(e.Repo)
	}
	return "repository does not exist: " + string(e.Repo)
}

// IsRepoNotExist reports if err is a RepoNotExistError.
func IsRepoNotExist(err error) bool {
	_, ok := err.(*RepoNotExistError)
	return ok
}

// IsCloneInProgress reports if err is a RepoNotExistError which has a clone
// in progress.
func IsCloneInProgress(err error) bool {
	if e, ok := err.(*RepoNotExistError); ok {
		return e.CloneInProgress
	}
	return false
}
