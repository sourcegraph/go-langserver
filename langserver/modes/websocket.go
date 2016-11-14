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

func WebSocket(addr string, connOpt []jsonrpc2.ConnOpt) error {
	log.Printf("langserver: websocket listening on: %s", addr)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("langserver-go: wsConn upgrading - w: %p, r: %p", &w, r)

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("langserver: wsConn upgrade error - w: %p, r: %p, err: %v", &w, r, err)
			return
		}

		log.Printf("langserver: wsConn: %p - upgraded - w: %p, r: %p", wsConn, &w, r)
		wsHandler(w, r, wsConn, connOpt)
	})
	
	err := http.ListenAndServe(addr, nil)
	return err
}

func wsHandler(w http.ResponseWriter, r *http.Request, wsConn *websocket.Conn, connOpt []jsonrpc2.ConnOpt) {
	// defer wsConn.Close()

	handler := langserver.NewHandler()
	for {
		messageType, reader, err := wsConn.NextReader()
		if err != nil {
			log.Printf("langserver: wsConn: %p - NextReader error - err: %v", wsConn, err)
			return
		}
		log.Printf("langserver: wsConn: %p - NextReader - reader: %p, messageType: %d", wsConn, &reader, messageType)

		writer, err := wsConn.NextWriter(messageType)
		if err != nil {
			log.Printf("langserver: wsConn: %p - NextWriter error - err: %v", wsConn, err)
			return
		}
		log.Printf("langserver: wsConn: %p - NextWriter - writer: %p", wsConn, &writer)

		rwc := wsrwc{reader: reader, writer: writer, closer: writer}
		conn := jsonrpc2.NewConn(ctx, rwc, handler, connOpt...)
		<-conn.DisconnectNotify()
		// defer conn.Close()
		// conn.ReadMessagesStart(ctx, rwc)

		// 	err = conn.Close()
		// 	if err != nil {
		// 		log.Printf("langserver-go: wsConn: %p - conn.Close() error - conn: %p, err: %v", wsConn, conn, err)
		// 		return
		// 	}
	}
}

type wsrwc struct {
	reader io.Reader
	writer io.Writer
	closer io.Closer
}

func (ws wsrwc) Read(p []byte) (int, error) {
	return ws.reader.Read(p)
}

func (ws wsrwc) Write(p []byte) (int, error) {
	return ws.writer.Write(p)
}

func (ws wsrwc) Close() error {
	return ws.closer.Close()
}
