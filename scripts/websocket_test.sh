#!/bin/sh -x

echo "Starting langserver-antha"
langserver-antha -mode ws -addr :9999 -trace &

echo "Testing WebSocket connection:"
node ./scripts/websocket_test
