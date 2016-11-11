package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/websocket"

	"github.com/sourcegraph/go-langserver/langserver"
	"github.com/sourcegraph/jsonrpc2"
)

var (
	mode    = flag.String("mode", "ws", "communication mode (stdio|tcp|ws)")
	addr    = flag.String("addr", ":4389", "server listen address (tcp|ws)")
	trace   = flag.Bool("trace", false, "print all requests and responses")
	logfile = flag.String("logfile", "", "also log to this file (in addition to stderr)")
)

func main() {
	flag.Parse()
	log.SetFlags(0)

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var logW io.Writer
	if *logfile == "" {
		logW = os.Stderr
	} else {
		f, err := os.Create(*logfile)
		if err != nil {
			return err
		}
		defer f.Close()
		logW = io.MultiWriter(os.Stderr, f)
	}
	log.SetOutput(logW)

	var connOpt []jsonrpc2.ConnOpt
	if *trace {
		connOpt = append(connOpt, jsonrpc2.LogMessages(log.New(logW, "", 0)))
	}

	switch *mode {
	case "tcp":
		lis, err := net.Listen("tcp", *addr)
		if err != nil {
			return err
		}
		defer lis.Close()

		log.Println("langserver-go: listening on", *addr)
		for {
			conn, err := lis.Accept()
			if err != nil {
				return err
			}
			jsonrpc2.NewConn(context.Background(), conn, langserver.NewHandler(), connOpt...)
		}

	case "ws":
		log.Println("langserver-go: websocket listening on", *addr)
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			echoHandler(w, r, connOpt)
		})
		err := http.ListenAndServe(*addr, nil)
		return err

	case "stdio":
		log.Println("langserver-go: reading on stdin, writing on stdout")
		<-jsonrpc2.NewConn(context.Background(), stdrwc{}, langserver.NewHandler(), connOpt...).DisconnectNotify()
		log.Println("connection closed")
		return nil

	default:
		return fmt.Errorf("invalid mode %q", *mode)
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func echoHandler(w http.ResponseWriter, r *http.Request, connOpt []jsonrpc2.ConnOpt) {
	log.Printf("langserver-go: wsConn upgrading - w: %p, r: %p", &w, r)

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("langserver-go: wsConn upgrade error - w: %p, r: %p, err: %v", &w, r, err)
		return
	}
	log.Printf("langserver-go: wsConn: %p - upgraded - w: %p, r: %p", wsConn, &w, r)

	// serve the client now
	for {
		messageType, reader, err := wsConn.NextReader()
		log.Printf("langserver-go: wsConn: %p - ReadMessage - messageType: %d", wsConn, messageType)
		if err != nil {
			log.Printf("langserver-go: wsConn: %p - ReadMessage error - err: %v", wsConn, err)
			return
		}

		writer, err := wsConn.NextWriter(messageType)
		if err != nil {
			log.Printf("langserver-go: wsConn: %p - NextWriter error - err: %v", wsConn, err)
			return
		}

		conn := &wsrwc{reader: reader, writer: writer, closer: writer}
		jsonrpc2.NewConn(context.Background(), conn, langserver.NewHandler(), connOpt...)
	}
}

type wsrwc struct {
	reader io.Reader
	writer io.Writer
	closer io.Closer
}

func (ws *wsrwc) Read(p []byte) (int, error) {
	return ws.reader.Read(p)
}

func (ws *wsrwc) Write(p []byte) (int, error) {
	return ws.writer.Write(p)
}

func (ws *wsrwc) Close() error {
	return ws.closer.Close()
}

type stdrwc struct{}

func (stdrwc) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (stdrwc) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (stdrwc) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}
	return os.Stdout.Close()
}
