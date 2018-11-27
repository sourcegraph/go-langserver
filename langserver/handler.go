package langserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"golang.org/x/tools/refactor/importgraph"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/go-lsp/lspext"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/sourcegraph/go-langserver/langserver/util"
)

// NewHandler creates a Go language server handler.
func NewHandler(defaultCfg Config) jsonrpc2.Handler {
	return lspHandler{jsonrpc2.HandlerWithError((&LangHandler{
		DefaultConfig: defaultCfg,
		HandlerShared: &HandlerShared{},
	}).handle)}
}

// lspHandler wraps LangHandler to correctly handle requests in the correct
// order.
//
// The LSP spec dictates a strict ordering that requests should only be
// processed serially in the order they are received. However, implementations
// are allowed to do concurrent computation if it doesn't affect the
// result. We actually can return responses out of order, since vscode does
// not seem to have issues with that. We also do everything concurrently,
// except methods which could mutate the state used by our typecheckers (ie
// textDocument/didOpen, etc). Those are done serially since applying them out
// of order could result in a different textDocument.
type lspHandler struct {
	jsonrpc2.Handler
}

// Handle implements jsonrpc2.Handler
func (h lspHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if isFileSystemRequest(req.Method) {
		h.Handler.Handle(ctx, conn, req)
		return
	}
	go h.Handler.Handle(ctx, conn, req)
}

// LangHandler is a Go language server LSP/JSON-RPC handler.
type LangHandler struct {
	mu sync.Mutex
	HandlerCommon
	*HandlerShared
	init *InitializeParams // set by "initialize" request

	typecheckCache   cache
	symbolCache      cache
	diagnosticsCache *diagnosticsCache

	// linter is the linter implementation that will be used.
	// linter can be nil.
	linter Linter

	// cache the reverse import graph. The sync.Once is a pointer since it
	// is reset when we reset caches. If it was a value we would racily
	// updated the internal mutex when assigning a new sync.Once.
	importGraphOnce *sync.Once
	importGraph     importgraph.Graph

	cancel *cancel

	// DefaultConfig is the default values used for configuration. It is
	// combined with InitializationOptions after initialize. This should be
	// set by LangHandler creators. Please read config instead.
	DefaultConfig Config

	// config is the language handler configuration. It is a combination of
	// DefaultConfig and InitializationOptions.
	config *Config // pointer so we panic if someone reads before we set it.
}

// reset clears all internal state in h.
func (h *LangHandler) reset(init *InitializeParams) error {
	for _, k := range init.Capabilities.TextDocument.Completion.CompletionItemKind.ValueSet {
		if k == lsp.CIKConstant {
			CIKConstantSupported = lsp.CIKConstant
			break
		}
	}

	if util.IsURI(lsp.DocumentURI(init.InitializeParams.RootPath)) {
		log.Printf("Passing an initialize rootPath URI (%q) is deprecated. Use rootUri instead.", init.InitializeParams.RootPath)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.HandlerCommon.Reset(init.Root()); err != nil {
		return err
	}
	if !h.HandlerShared.Shared {
		// Only reset the shared data if this lang server is running
		// by itself.
		if err := h.HandlerShared.Reset(!init.NoOSFileSystemAccess); err != nil {
			return err
		}
	}
	config := h.DefaultConfig.Apply(init.InitializationOptions)
	h.config = &config
	h.init = init
	h.cancel = &cancel{}
	h.resetCaches(false)
	return nil
}

func (h *LangHandler) resetCaches(lock bool) {
	if lock {
		h.mu.Lock()
	}

	h.importGraphOnce = &sync.Once{}
	h.importGraph = nil

	if h.typecheckCache == nil {
		h.typecheckCache = newTypecheckCache()
	} else {
		h.typecheckCache.Purge()
	}

	if h.symbolCache == nil {
		h.symbolCache = newSymbolCache()
	} else {
		h.symbolCache.Purge()
	}

	if h.diagnosticsCache == nil {
		h.diagnosticsCache = newDiagnosticsCache()
	}

	if lock {
		h.mu.Unlock()
	}
}

// handle implements jsonrpc2.Handler.
func (h *LangHandler) handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (result interface{}, err error) {
	return h.Handle(ctx, conn, req)
}

// Handle creates a response for a JSONRPC2 LSP request. Note: LSP has strict
// ordering requirements, so this should not just be wrapped in an
// jsonrpc2.AsyncHandler. Ensure you have the same ordering as used in the
// NewHandler implementation.
func (h *LangHandler) Handle(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request) (result interface{}, err error) {
	// Prevent any uncaught panics from taking the entire server down.
	defer func() {
		if perr := util.Panicf(recover(), "%v", req.Method); perr != nil {
			err = perr
		}
	}()

	var cancelManager *cancel
	h.mu.Lock()
	cancelManager = h.cancel
	if req.Method != "initialize" && h.init == nil {
		h.mu.Unlock()
		return nil, errors.New("server must be initialized")
	}
	h.mu.Unlock()
	if err := h.CheckReady(); err != nil {
		if req.Method == "exit" {
			err = nil
		}
		return nil, err
	}

	if conn, ok := conn.(*jsonrpc2.Conn); ok && conn != nil {
		h.InitTracer(conn)
	}
	span, ctx, err := h.SpanForRequest(ctx, "lang", req, opentracing.Tags{"mode": "go"})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			ext.Error.Set(span, true)
			span.LogEvent(fmt.Sprintf("error: %v", err))
		}
		span.Finish()
	}()

	// Notifications don't have an ID, so they can't be cancelled
	if cancelManager != nil && !req.Notif {
		var cancel func()
		ctx, cancel = cancelManager.WithCancel(ctx, req.ID)
		defer cancel()
	}

	switch req.Method {
	case "initialize":
		if h.init != nil {
			return nil, errors.New("language server is already initialized")
		}
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params InitializeParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}

		// HACK: RootPath is not a URI, but historically we treated it
		// as such. Convert it to a file URI
		if params.RootPath != "" && !util.IsURI(lsp.DocumentURI(params.RootPath)) {
			params.RootPath = string(util.PathToURI(params.RootPath))
		}

		if err := h.reset(&params); err != nil {
			return nil, err
		}

		// PERF: Kick off a workspace/symbol in the background to warm up the server
		if yes, _ := strconv.ParseBool(envWarmupOnInitialize); yes {
			go func() {
				ctx, cancel := context.WithDeadline(ctx, time.Now().Add(30*time.Second))
				defer cancel()
				_, _ = h.handleWorkspaceSymbol(ctx, conn, req, lspext.WorkspaceSymbolParams{
					Query: "",
					Limit: 100,
				})
			}()
		}

		// set the configured linter
		if h.config.DiagnosticsEnabled && h.config.LintTool != lintToolNone {
			switch h.config.LintTool {
			case lintToolGolint:
				h.linter = golint{}
			}

			if h.linter == nil {
				log.Printf("warning: lint tool %s not supported", h.config.LintTool)
			} else if err := h.linter.IsInstalled(ctx, h.BuildContext(ctx)); err != nil {
				h.linter = nil
				log.Printf("warning: lint tool (%s) initialize err: %s", h.config.LintTool, err)
			} else {
				// kick off a lint of the entire workspace
				go func() {
					ctx, cancel := context.WithDeadline(ctx, time.Now().Add(30*time.Second))
					defer cancel()
					err := h.lintWorkspace(ctx, h.BuildContext(ctx), conn)
					if err != nil {
						log.Printf("warning: failed to lint workspace: %s", err)
					}
				}()
			}
		}

		kind := lsp.TDSKIncremental
		var completionOp *lsp.CompletionOptions
		if h.config.GocodeCompletionEnabled {
			completionOp = &lsp.CompletionOptions{TriggerCharacters: []string{"."}}
		}
		return lsp.InitializeResult{
			Capabilities: lsp.ServerCapabilities{
				TextDocumentSync: &lsp.TextDocumentSyncOptionsOrKind{
					Kind: &kind,
				},
				CompletionProvider:           completionOp,
				DefinitionProvider:           true,
				TypeDefinitionProvider:       true,
				DocumentFormattingProvider:   true,
				DocumentSymbolProvider:       true,
				HoverProvider:                true,
				ReferencesProvider:           true,
				RenameProvider:               true,
				WorkspaceSymbolProvider:      true,
				ImplementationProvider:       true,
				XWorkspaceReferencesProvider: true,
				XDefinitionProvider:          true,
				XWorkspaceSymbolByProperties: true,
				SignatureHelpProvider:        &lsp.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
			},
		}, nil

	case "initialized":
		// A notification that the client is ready to receive requests. Ignore
		return nil, nil

	case "shutdown":
		h.ShutDown()
		return nil, nil

	case "exit":
		if c, ok := conn.(*jsonrpc2.Conn); ok {
			c.Close()
		}
		return nil, nil

	case "$/cancelRequest":
		// notification, don't send back results/errors
		if req.Params == nil {
			return nil, nil
		}
		var params lsp.CancelParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, nil
		}
		if cancelManager == nil {
			return nil, nil
		}
		cancelManager.Cancel(jsonrpc2.ID{
			Num:      params.ID.Num,
			Str:      params.ID.Str,
			IsString: params.ID.IsString,
		})
		return nil, nil

	case "textDocument/hover":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleHover(ctx, conn, req, params)

	case "textDocument/definition":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleDefinition(ctx, conn, req, params)

	case "textDocument/typeDefinition":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleTypeDefinition(ctx, conn, req, params)

	case "textDocument/xdefinition":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleXDefinition(ctx, conn, req, params)

	case "textDocument/completion":
		if !h.config.GocodeCompletionEnabled {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound,
				Message: fmt.Sprintf("completion is disabled. Enable with flag `-gocodecompletion`")}
		}
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.CompletionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleTextDocumentCompletion(ctx, conn, req, params)

	case "textDocument/references":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.ReferenceParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleTextDocumentReferences(ctx, conn, req, params)

	case "textDocument/implementation":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleTextDocumentImplementation(ctx, conn, req, params)

	case "textDocument/documentSymbol":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.DocumentSymbolParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleTextDocumentSymbol(ctx, conn, req, params)

	case "textDocument/signatureHelp":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleTextDocumentSignatureHelp(ctx, conn, req, params)

	case "textDocument/formatting":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.DocumentFormattingParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleTextDocumentFormatting(ctx, conn, req, params)

	case "workspace/symbol":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lspext.WorkspaceSymbolParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleWorkspaceSymbol(ctx, conn, req, params)

	case "workspace/xreferences":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lspext.WorkspaceReferencesParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleWorkspaceReferences(ctx, conn, req, params)

	case "textDocument/rename":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.RenameParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.handleRename(ctx, conn, req, params)
	default:
		if isFileSystemRequest(req.Method) {
			uri, fileChanged, err := h.handleFileSystemRequest(ctx, req)
			if fileChanged {
				// a file changed, so we must re-typecheck and re-enumerate symbols
				h.resetCaches(true)
			}
			if uri != "" {
				// a user is viewing this path, hint to add it to the cache
				// (unless we're primarily using binary package cache .a
				// files).
				if !h.config.UseBinaryPkgCache || (h.config.DiagnosticsEnabled && req.Method == "textDocument/didSave") {
					go h.typecheck(ctx, conn, uri, lsp.Position{})
				}

				if h.config.DiagnosticsEnabled && h.linter != nil && req.Method == "textDocument/didSave" {
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						err := h.lintPackage(ctx, h.BuildContext(ctx), conn, uri)
						if err != nil {
							log.Printf("warning: failed to lint package: %s", err)
						}
					}()
				}
			}
			return nil, err
		}

		return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: fmt.Sprintf("method not supported: %s", req.Method)}
	}
}
