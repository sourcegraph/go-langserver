package modes

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/sourcegraph/go-langserver/langserver"
	"github.com/sourcegraph/jsonrpc2"
)

var (
	ctx      = context.Background()
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// disable security for now
		// "If CheckOrigin returns false, you will get the error you described. By default, it returns false if the request is cross-origin."
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

// WebSocket listener on addr with connOpts.
func WebSocket(addr string, connOpt []jsonrpc2.ConnOpt) error {
	log.Printf("ws ======== langserver-go: websocket listening on: %s", addr)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("ws ======== langserver-go: wsConn upgrading - w: %p, r: %p", &w, r)

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws ======== langserver-go: wsConn upgrade error - w: %p, r: %p, err: %v", &w, r, err)
			return
		}

		log.Printf("ws ======== langserver-go: wsConn: %p - upgraded - w: %p, r: %p", wsConn, &w, r)
		webSocketHandler(w, r, wsConn, connOpt)
	})

	err := http.ListenAndServe(addr, nil)
	return err
}

func webSocketHandler(w http.ResponseWriter, r *http.Request, wsConn *websocket.Conn, connOpt []jsonrpc2.ConnOpt) {
	defer wsConn.Close()

	handler := langserver.NewHandler()
	for {
		reader, writer, messageType, err := getReaderWriter(wsConn)
		if err != nil {
			log.Printf("ws ======== langserver-go: getReaderWriter failed - wsConn     : %p", wsConn)
			log.Printf("ws ======== langserver-go: getReaderWriter        - err        : %v", err)
			log.Printf("ws ======== langserver-go: getReaderWriter        - messageType: %v", messageType)
			break
		}

		rwc := webSocketReadWriteCloser{reader: reader, writer: writer, closer: writer}
		codec := jsonrpc2.VSCodeObjectCodec{}
		stream := jsonrpc2.NewBufferedStream(rwc, codec)
		jsonrpc2.NewConn(
			ctx,
			stream,
			handler,
			connOpt...)

		if err := writer.Close(); err != nil {
			log.Printf("ws ======== langserver-go: writer.Close() failed  - wsCon      : %p", wsConn)
			log.Printf("ws ======== langserver-go: writer.Close() failed  - err        : %v", err)
			break
		}
	}

	log.Printf("ws ======== langserver-go: wsConn: %p - done", wsConn)
}

func getReaderWriter(wsConn *websocket.Conn) (reader io.Reader, writer io.WriteCloser, messageType int, err error) {
	messageType, reader, err = wsConn.NextReader()
	if err != nil {
		log.Printf("ws ======== langserver-go: wsConn: %p - NextReader err: %v", wsConn, err)
		return
	}
	log.Printf("ws ======== langserver-go: wsConn: %p - NextReader: %p", wsConn, &reader)

	writer, err = wsConn.NextWriter(messageType)
	if err != nil {
		log.Printf("ws ======== langserver-go: wsConn: %p - NextWriter err: %v", wsConn, err)
		return
	}
	log.Printf("ws ======== langserver-go: wsConn: %p - NextWriter: %p", wsConn, &writer)

	return
}

type webSocketReadWriteCloser struct {
	reader io.Reader
	writer io.Writer
	closer io.Closer
}

func (ws webSocketReadWriteCloser) Read(p []byte) (int, error) {
	return ws.reader.Read(p)
}

func (ws webSocketReadWriteCloser) Write(p []byte) (int, error) {
	return ws.writer.Write(p)
}

func (ws webSocketReadWriteCloser) Close() error {
	return ws.closer.Close()
}
