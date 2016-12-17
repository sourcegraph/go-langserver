
#!/bin/sh -x

# get the go-langserver
go get github.com/sourcegraph/go-langserver/langserver/cmd/langserver

# setup remote tracking branches
cd $GOPATH/src/github.com/sourcegraph/go-langserver
git remote add mbana git@github.com:mbana/go-langserver.git
git fetch origin
git fetch mbana
git checkout --track -b mbana/master mbana/master
git merge origin/master

# do the build for both langserver-go and langserver-anth
go install -x -v -a github.com/sourcegraph/go-langserver/langserver/cmd/langserver-{go,antha}
ls -lah `which langserver-{go,antha}`

# ( \
#   go install -x -v -a github.com/sourcegraph/go-langserver/langserver/cmd/langserver-antha && \
#   ls -lah `which langserver-antha` && \
#   langserver-antha -mode ws -trace \
# )

( \
  go install -x -v -a -race github.com/sourcegraph/go-langserver/langserver/cmd/langserver-antha && \
  ls -lah `which langserver-antha` && \
  langserver-antha -mode ws -trace \
)
