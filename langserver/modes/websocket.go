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
	log.Printf("ws ======== langserver-go: websocket listening on: %s", addr)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("ws ======== langserver-go: conn upgrade 					- w: %p, r: %p", &w, r)

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws ======== langserver-go: upgrade err  conn: %p", conn)
			log.Printf("ws ======== langserver-go: upgraded err    w: %p", &w)
			log.Printf("ws ======== langserver-go: upgraded err    r: %p", r)
			return
		}

		log.Printf("ws ======== langserver-go: upgraded     conn: %p", conn)
		log.Printf("ws ======== langserver-go: upgraded        w: %p", &w)
		log.Printf("ws ======== langserver-go: upgraded        r: %p", r)

		webSocketHandler(w, r, conn, connOpt)

		log.Printf("ws ======== langserver-go: done         conn: %p", conn)
		log.Printf("ws ======== langserver-go: done            w: %p", &w)
		log.Printf("ws ======== langserver-go: done            r: %p", r)
	})

	err = http.ListenAndServe(addr, nil)

	return
}

func webSocketHandler(w http.ResponseWriter, r *http.Request, conn *websocket.Conn, connOpt []jsonrpc2.ConnOpt) {
	for {
		stream := jsonrpc2ws.NewObjectStream(conn, jsonrpc2.VSCodeObjectCodec{})
		conn := jsonrpc2.NewConn(
			ctx,
			stream,
			langserver.NewHandler(),
			connOpt...)
		<-conn.DisconnectNotify()
	}
}
