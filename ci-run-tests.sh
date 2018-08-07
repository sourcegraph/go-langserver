#!/bin/bash

# Temporary script to kill slow test runs with SIGABRT so we can see where it
# got stuck.

# Script adapted from
# http://www.bashcookbook.com/bashinfo/source/bash-4.0/examples/scripts/timeout3

(
    ((t = 300))

    while ((t > 0)); do
        sleep 1
        kill -0 $$ || exit 0
        ((t -= 1))
    done

    kill -s ABRT $$
) 2> /dev/null &

exec go test -race -v ./...
