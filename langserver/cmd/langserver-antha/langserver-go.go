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
        http.HandleFunc("/", echoHandler)
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
 
func echoHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("langserver-go: conn upgrading - w: %p, r: %p", &w, r)

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
		log.Printf("langserver-go: conn upgrade error - w: %p, r: %p, err: %v", &w, r, err)
        return
    }
 	log.Printf("langserver-go: conn: %p - upgraded - w: %p, r: %p", conn, &w, r)

	// serve the client now
    for {
    	messageType, reader, err := conn.NextReader()
		log.Printf("langserver-go: conn: %p - ReadMessage - messageType: %d", conn, messageType)
        if err != nil {
			log.Printf("langserver-go: conn: %p - ReadMessage error - err: %v", conn, err)
            return
        }

		writer, err := conn.NextWriter(messageType)
		if err != nil {
			log.Printf("langserver-go: conn: %p - NextWriter error - err: %v", conn, err)
			return
		}

		written, err := io.Copy(writer, reader);
		if err != nil {
			log.Printf("langserver-go: conn: %p - io.Copy() error - err: %v", conn, err)
			return
		}
		log.Printf("langserver-go: conn: %p - io.Copy() - written: %v", conn, written)

		if err := writer.Close(); err != nil {
			log.Printf("langserver-go: conn: %p - writer.Close() error - err: %v", conn, err)
			return
		}
    }
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
