#!/bin/sh -x

PORT_WEBSOCKET="4389"

# start server
langserver-antha -mode ws -addr :"$PORT_WEBSOCKET" -trace &

# jobs -p
SERVER_JOB=$(echo $!)

trap "kill -9 $SERVER_JOB" EXIT

# sleep a bit then start client tests
sleep 2s
PORT_WEBSOCKET=$PORT_WEBSOCKET node ./tests/js/out/main

