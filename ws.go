package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
)

// A ObjectStream is a jsonrpc2.ObjectStream that uses a WebSocket to
// send and receive JSON-RPC 2.0 objects.
type ObjectStream struct {
	conn *websocket.Conn
}

// NewObjectStream creates a new jsonrpc2.ObjectStream for sending and
// receiving JSON-RPC 2.0 objects over a WebSocket.
func NewObjectStream(conn *websocket.Conn) ObjectStream {
	return ObjectStream{conn: conn}
}

// WriteObject implements jsonrpc2.ObjectStream.
func (t ObjectStream) WriteObject(obj interface{}) error {
	objBytes, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	t.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(objBytes), objBytes)))
	return t.conn.WriteJSON(obj)
}

// ReadObject implements jsonrpc2.ObjectStream.
func (t ObjectStream) ReadObject(v interface{}) error {
	_, message, err := t.conn.ReadMessage()
	if e, ok := err.(*websocket.CloseError); ok {
		if e.Code == websocket.CloseAbnormalClosure && e.Text == io.ErrUnexpectedEOF.Error() {
			// Suppress a noisy (but harmless) log message by
			// unwrapping this error.
			err = io.ErrUnexpectedEOF
		}
	}
	scanner := bufio.NewReader(bytes.NewReader(message))
	scanner.ReadString('\n')
	scanner.ReadString('\n')
	return json.NewDecoder(scanner).Decode(v)
}

// Close implements jsonrpc2.ObjectStream.
func (t ObjectStream) Close() error {
	return t.conn.Close()
}