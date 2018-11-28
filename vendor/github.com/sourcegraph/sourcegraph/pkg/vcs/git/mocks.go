package git

import (
	"os"

	"github.com/sourcegraph/sourcegraph/pkg/api"
)

// Mocks is used to mock behavior in tests. Tests must call ResetMocks() when finished to ensure its
// mocks are not (inadvertently) used by subsequent tests.
//
// (The emptyMocks is used by ResetMocks to zero out Mocks without needing to use a named type.)
var Mocks, emptyMocks struct {
	GetCommit        func(api.CommitID) (*Commit, error)
	ExecSafe         func(params []string) (stdout, stderr []byte, exitCode int, err error)
	RawLogDiffSearch func(opt RawLogDiffSearchOptions) ([]*LogCommitSearchResult, bool, error)
	ReadDir          func(commit api.CommitID, name string, recurse bool) ([]os.FileInfo, error)
	ResolveRevision  func(spec string, opt *ResolveRevisionOptions) (api.CommitID, error)
	Stat             func(commit api.CommitID, name string) (os.FileInfo, error)
}

// ResetMocks clears the mock functions set on Mocks (so that subsequent tests don't inadvertently
// use them).
func ResetMocks() {
	Mocks = emptyMocks
}
