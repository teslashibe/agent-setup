package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

// JSON-RPC 2.0 error codes (https://www.jsonrpc.org/specification#error_object)
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603

	// MCP-specific application error codes (must be in -32000..-32099 per spec)
	codeUnknownTool        = -32010
	codeCredentialMissing  = -32011
	codeCredentialInvalid  = -32012
	codeUnauthorized       = -32001
)

// Transport mounts the JSON-RPC 2.0 over HTTP transport for the MCP server.
//
// Endpoints:
//
//	POST /mcp/v1            JSON-RPC 2.0 request (initialize, tools/list, tools/call)
//	GET  /mcp/v1/health     Lightweight liveness probe (no auth)
//
// JWT authentication is applied by the parent route group.
type Transport struct{ srv *Server }

// NewTransport constructs a Transport bound to the given Server.
func NewTransport(srv *Server) *Transport { return &Transport{srv: srv} }

// Mount registers the routes on r. Caller is responsible for applying
// authentication middleware before calling Mount; the only unauthenticated
// route is /v1/health, which is registered separately on the parent app.
func (t *Transport) Mount(r fiber.Router) {
	r.Post("/v1", t.handleRPC)
	r.Get("/v1", t.rejectGet) // friendly error for browser pings
}

// MountHealth registers the unauthenticated health probe.
func (t *Transport) MountHealth(r fiber.Router) {
	r.Get("/v1/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":   "ok",
			"protocol": "mcp",
			"version":  "0.1.0",
			"tools":    len(t.srv.registry.Tools()),
		})
	})
}

func (t *Transport) rejectGet(c *fiber.Ctx) error {
	return c.Status(fiber.StatusMethodNotAllowed).JSON(fiber.Map{
		"error": "MCP endpoint accepts JSON-RPC 2.0 over POST. See /mcp/v1/health for status.",
	})
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (t *Transport) handleRPC(c *fiber.Ctx) error {
	userID := apperrors.UserID(c)
	if userID == "" {
		return jsonRPCError(c, nil, codeUnauthorized, "unauthorized", nil)
	}

	body := c.Body()
	if len(body) == 0 {
		return jsonRPCError(c, nil, codeInvalidRequest, "empty request body", nil)
	}

	// Support batch requests (array of RPCs). MCP rarely uses batching but the
	// JSON-RPC 2.0 spec requires it; we serve each call against the same
	// per-request client cache.
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		var batch []rpcRequest
		if err := json.Unmarshal(body, &batch); err != nil {
			return jsonRPCError(c, nil, codeParseError, "invalid JSON: "+err.Error(), nil)
		}
		ctx := WithRequest(c.UserContext())
		out := make([]rpcResponse, 0, len(batch))
		for _, req := range batch {
			out = append(out, t.dispatch(ctx, userID, req))
		}
		return c.JSON(out)
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return jsonRPCError(c, nil, codeParseError, "invalid JSON: "+err.Error(), nil)
	}
	ctx := WithRequest(c.UserContext())
	resp := t.dispatch(ctx, userID, req)
	return c.JSON(resp)
}

func (t *Transport) dispatch(ctx fiberCtx, userID string, req rpcRequest) rpcResponse {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return errResp(req.ID, codeInvalidRequest, "jsonrpc must be \"2.0\"", nil)
	}
	switch req.Method {
	case "initialize":
		return t.handleInitialize(req)
	case "tools/list":
		return t.handleToolsList(req)
	case "tools/call":
		return t.handleToolsCall(ctx, userID, req)
	case "ping":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{"pong": true}}
	default:
		return errResp(req.ID, codeMethodNotFound, fmt.Sprintf("unknown method %q", req.Method), nil)
	}
}

// fiberCtx is a tiny alias used in dispatch signatures so we can swap the
// concrete context implementation without churning every internal helper.
type fiberCtx = context.Context

func (t *Transport) handleInitialize(req rpcRequest) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: fiber.Map{
			"protocolVersion": "2024-11-05",
			"serverInfo": fiber.Map{
				"name":    "agent-setup",
				"version": "0.1.0",
			},
			"capabilities": fiber.Map{
				"tools": fiber.Map{},
			},
		},
	}
}

func (t *Transport) handleToolsList(req rpcRequest) rpcResponse {
	descs := t.srv.ListTools()
	tools := make([]fiber.Map, len(descs))
	for i, d := range descs {
		tools[i] = fiber.Map{
			"name":         d.Name,
			"description":  d.Description,
			"inputSchema":  d.InputSchema,
			"_meta": fiber.Map{
				"platform":     d.Platform,
				"wraps_method": d.WrapsMethod,
				"tags":         d.Tags,
			},
		}
	}
	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{"tools": tools}}
}

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (t *Transport) handleToolsCall(ctx fiberCtx, userID string, req rpcRequest) rpcResponse {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResp(req.ID, codeInvalidParams, "invalid params: "+err.Error(), nil)
	}
	if strings.TrimSpace(params.Name) == "" {
		return errResp(req.ID, codeInvalidParams, "tool name is required", nil)
	}
	if len(params.Arguments) == 0 {
		params.Arguments = json.RawMessage(`{}`)
	}

	res := t.srv.CallTool(ctx, userID, params.Name, params.Arguments)
	if res.Error != nil {
		code := codeInternalError
		switch res.Error.Code {
		case "unknown_tool":
			code = codeUnknownTool
		case "credential_missing":
			code = codeCredentialMissing
		case "credential_invalid", "credential_unreadable", "binding_misconfigured":
			code = codeCredentialInvalid
		}
		return errResp(req.ID, code, res.Error.Message, fiber.Map{
			"code":      res.Error.Code,
			"retryable": res.Error.Retryable,
			"data":      res.Error.Data,
			"tool":      res.Tool,
			"platform":  res.Platform,
		})
	}

	// MCP's tools/call returns a content array. We wrap the structured output
	// as a single text block (compact JSON) so any MCP-compliant agent can
	// parse it. The structured result is also surfaced in _meta for richer
	// agent introspection.
	payload, err := compactJSON(res.Output)
	if err != nil {
		return errResp(req.ID, codeInternalError, "failed to encode tool output: "+err.Error(), nil)
	}
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: fiber.Map{
			"content": []fiber.Map{
				{"type": "text", "text": string(payload)},
			},
			"isError": false,
			"_meta": fiber.Map{
				"tool":     res.Tool,
				"platform": res.Platform,
				"output":   res.Output,
			},
		},
	}
}

func errResp(id json.RawMessage, code int, msg string, data any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg, Data: data}}
}

func jsonRPCError(c *fiber.Ctx, id json.RawMessage, code int, msg string, data any) error {
	resp := errResp(id, code, msg, data)
	return c.JSON(resp)
}
