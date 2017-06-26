// package lspserver implements a general LSP server.
package lspserver

import (
	"context"
	"sync"

	"github.com/sourcegraph/jsonrpc2"
)

type Handler struct {
	Init func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) jsonrpc2.Handler

	mu sync.Mutex
	h  jsonrpc2.Handler
}

func (h *Handler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	switch req.Method {
	case "initialize":
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.h != nil {
			conn.SendResponse(ctx, &jsonrpc2.Response{
				ID: req.ID,
				Error: &jsonrpc2.Error{
					Code:    jsonrpc2.CodeInvalidRequest,
					Message: "language server is already initialized",
				}})
			return
		}
		h.h = h.Init(ctx, conn, req)

	case "shutdown":
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.h == nil {
			conn.SendResponse(ctx, &jsonrpc2.Response{
				ID: req.ID,
				Error: &jsonrpc2.Error{
					Code:    jsonrpc2.CodeInvalidRequest,
					Message: "language server is not initialized",
				}})
			return
		}
		h.h.Handle(ctx, conn, req)
		h.h = nil

	case "exit":
		conn.Close()

	default:
		h.mu.Lock()
		h2 := h
		h.mu.Unlock()
		if h2 == nil {
			conn.SendResponse(ctx, &jsonrpc2.Response{
				ID: req.ID,
				Error: &jsonrpc2.Error{
					Code:    jsonrpc2.CodeInvalidRequest,
					Message: "language server is not initialized",
				}})
			return
		}
		h2.Handle(ctx, conn, req)
	}
}
