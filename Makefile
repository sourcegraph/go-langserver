BUILD_IMAGE := golang:1.10.3
ARCH := amd64
PKG := github.com/sourcegraph/go-langserver
SRC_DIRS := pkg langserver

docker-test: make-dirs
	@chmod +x ./test.sh
	@docker run                                                             \
	    -ti                                                                 \
	    --rm                                                                \
	    -u $$(id -u):$$(id -g)                                              \
	    -v "$$(pwd)/.go:/go"                                                \
	    -v "$$(pwd):/go/src/$(PKG)"                                         \
	    -v "$$(pwd)/bin/$(ARCH):/go/bin"                                    \
	    -v "$$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static" \
	    -v "$$(pwd)/.go/cache:/.cache"                                      \
	    -w /go/src/$(PKG)                                                   \
	    $(BUILD_IMAGE)                                                      \
	    /bin/sh -c "                                                        \
	        ./test.sh $(SRC_DIRS)                                  			\
	    "
make-dirs:
	@mkdir -p bin/$(ARCH)
	@mkdir -p .go .go/cache .go/src/$(PKG) .go/pkg .go/bin .go/std/$(ARCH)
	
