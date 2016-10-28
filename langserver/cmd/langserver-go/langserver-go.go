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
    "golang.org/x/net/websocket"

	"github.com/sourcegraph/jsonrpc2"
	"github.com/sourcegraph/go-langserver/langserver"
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
        var sockets = make(map[string]*websocket.Conn)

		log.Println("langserver-go: ws - started")
        handler := websocket.Handler(func (ws *websocket.Conn) {

            id := ws.RemoteAddr().String() + "-" + ws.Request().RemoteAddr + "-" + ws.Request().UserAgent()
            sockets[id] = ws
            log.Println(id, "is waiting")
            <-jsonrpc2.NewConn(context.Background(), ws, langserver.NewHandler(), connOpt...).DisconnectNotify()

            log.Println(id, "is finished")
        })
        http.Handle("/echo", handler)
        err := http.ListenAndServe(*addr, nil)
	    log.Println("langserver-go: ws - ended")        
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
