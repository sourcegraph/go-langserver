package protocol

import (
	"time"

	"github.com/sourcegraph/sourcegraph/pkg/api"
)

// ExecRequest is a request to execute a command inside a git repository.
type ExecRequest struct {
	Repo api.RepoName `json:"repo"`

	// URL is the repository's Git remote URL. If the gitserver already has cloned the repository,
	// this field is optional (it will use the last-used Git remote URL). If the repository is not
	// cloned on the gitserver, the request will fail.
	URL string `json:"url,omitempty"`

	EnsureRevision string      `json:"ensureRevision"`
	Args           []string    `json:"args"`
	Opt            *RemoteOpts `json:"opt"`
}

// RemoteOpts configures interactions with a remote repository.
type RemoteOpts struct {
	SSH   *SSHConfig   `json:"ssh"`   // SSH configuration for communication with the remote
	HTTPS *HTTPSConfig `json:"https"` // HTTPS configuration for communication with the remote
}

// SSHConfig configures and authenticates SSH for communication with remotes.
type SSHConfig struct {
	User       string `json:"user,omitempty"`      // SSH user (if empty, inferred from URL)
	PublicKey  []byte `json:"publicKey,omitempty"` // SSH public key (if nil, inferred from PrivateKey)
	PrivateKey []byte `json:"privateKey"`          // SSH private key, usually passed to ssh.ParsePrivateKey (passphrases currently unsupported)
}

// HTTPSConfig configures and authenticates HTTPS for communication with remotes.
type HTTPSConfig struct {
	User string `json:"user"` // the username provided to the remote
	Pass string `json:"pass"` // the password provided to the remote
}

// RepoUpdateRequest is a request to update the contents of a given repo, or clone it if it doesn't exist.
type RepoUpdateRequest struct {
	Repo  api.RepoName  `json:"repo"`  // identifying URL for repo
	URL   string        `json:"url"`   // repo's remote URL
	Since time.Duration `json:"since"` // debounce interval for queries, used only with request-repo-update
}

// RepoUpdateResponse returns meta information of the repo enqueued for
// update.
//
// TODO just use RepoInfoResponse?
type RepoUpdateResponse struct {
	Cloned          bool
	CloneInProgress bool
	LastFetched     *time.Time
	LastChanged     *time.Time
	Error           string // an error reported by the update, as opposed to a protocol error
	QueueCap        int    // size of the clone queue
	QueueLen        int    // current clone operations
	// Following items likely provided only if the request specified waiting.
	Received *time.Time // time request was received by handler function
	Started  *time.Time // time request actually started processing
	Finished *time.Time // time request completed
}

type NotFoundPayload struct {
	CloneInProgress bool `json:"cloneInProgress"` // If true, exec returned with noop because clone is in progress.

	// CloneProgress is a progress message from the running clone command.
	CloneProgress string `json:"cloneProgress,omitempty"`
}

// IsRepoCloneableRequest is a request to determine if a repo is cloneable.
type IsRepoCloneableRequest struct {
	// Repo is the repository to check.
	Repo api.RepoName `json:"Repo"`

	// URL is the repository's Git remote URL.
	URL string `json:"url"`
}

// IsRepoCloneableResponse is the response type for the IsRepoCloneableRequest.
type IsRepoCloneableResponse struct {
	Cloneable bool   // whether the repo is cloneable
	Reason    string // if not cloneable, the reason why not
}

// IsRepoClonedRequest is a request to determine if a repo currently exists on gitserver.
type IsRepoClonedRequest struct {
	// Repo is the repository to check.
	Repo api.RepoName
}

// RepoInfoRequest is a request for information about a repository on gitserver.
type RepoInfoRequest struct {
	// Repo is the repository to get information about.
	Repo api.RepoName
}

// RepoDeleteRequest is a request to delete a repository clone on gitserver
type RepoDeleteRequest struct {
	// Repo is the repository to delete.
	Repo api.RepoName
}

// RepoInfoResponse is the response to a repository information request (RepoInfoRequest).
type RepoInfoResponse struct {
	URL             string     // this repository's Git remote URL
	CloneInProgress bool       // whether the repository is currently being cloned
	CloneProgress   string     // a progress message from the running clone command.
	Cloned          bool       // whether the repository has been cloned successfully
	LastFetched     *time.Time // when the last `git remote update` or `git fetch` occurred
	LastChanged     *time.Time // timestamp of the most recent ref in the git repository

	// CloneTime is the time the clone occurred. Note: Repositories may be
	// recloned automatically, so this time is likely to move forward
	// periodically.
	CloneTime *time.Time
}

// CreateCommitFromPatchRequest is the request information needed for creating
// the simulated staging area git object for a repo.
type CreateCommitFromPatchRequest struct {
	// Repo is the repository to get information about.
	Repo api.RepoName
	// BaseCommit is the revision that the staging area object is based on
	BaseCommit api.CommitID
	// Patch is the diff contents to be used to create the staging area revision
	Patch string
	// TargetRef is the ref that will be created for this patch
	TargetRef string
	// CommitInfo is the information that will be used when creating the commit from a patch
	CommitInfo PatchCommitInfo
}

// PatchCommitInfo will be used for commit information when creating a commit from a patch
type PatchCommitInfo struct {
	Message     string
	AuthorName  string
	AuthorEmail string
	Date        time.Time
}

// CreatePatchFromPatchResponse is the response type returned after creating
// a staging object for Phabricator
type CreatePatchFromPatchResponse struct {
	// Rev is the tag that the staging object can be found at
	Rev string
}
