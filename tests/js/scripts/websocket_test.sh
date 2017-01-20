#!/bin/sh -x

let PORT_WEBSOCKET="8888"

echo "Installing deps"
npm install

echo "Starting langserver-antha"
langserver-antha -mode ws -addr :${PORT_WEBSOCKET} -trace &

echo "Testing WebSocket connection:"
PORT_WEBSOCKET=${PORT_WEBSOCKET} node ./scripts/websocket_test
