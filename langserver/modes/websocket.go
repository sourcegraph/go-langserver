package modes

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/sourcegraph/go-langserver/langserver"
	"github.com/sourcegraph/jsonrpc2"
	jsonrpc2ws "github.com/sourcegraph/jsonrpc2/websocket"
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
func WebSocket(addr string, connOpt []jsonrpc2.ConnOpt) (err error) {
	log.Printf("listening on %s", addr)

	handler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("conn upgrade - w: %p, r: %p", &w, r)
		if conn, err := upgrader.Upgrade(w, r, nil); err == nil {
			log.Printf("upgrade ok - conn: %p, w: %p, r: %p", conn, &w, r)
			webSocketHandler(w, r, conn, connOpt)
		} else {
			log.Printf("upgrade err - err: %v, w: %p, r: %p", err, &w, r)
			return
		}
	}
	http.HandleFunc("/", handler)
	err = http.ListenAndServe(addr, nil)

	return
}

func webSocketHandler(w http.ResponseWriter, r *http.Request, wsConn *websocket.Conn, connOpt []jsonrpc2.ConnOpt) {
	for {
		stream := jsonrpc2ws.NewObjectStream(wsConn, jsonrpc2.VSCodeObjectCodec{})
		conn := jsonrpc2.NewConn(
			ctx,
			stream,
			langserver.NewHandler(),
			connOpt...)
		<-conn.DisconnectNotify()
		conn.Close()
		break
	}

	log.Printf("webSocketHandler:Closed - wsConn: %p", wsConn)
}
