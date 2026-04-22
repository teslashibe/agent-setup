package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/teslashibe/mcptool"

	"github.com/teslashibe/agent-setup/backend/internal/credentials"
)

// CredentialResolver returns the decrypted credential JSON for (user, platform).
// Implemented by *credentials.Service.
type CredentialResolver interface {
	Decrypted(ctx context.Context, userID, platform string) (json.RawMessage, error)
}

// Server is the transport-agnostic MCP request dispatcher. It looks up the
// requested tool, resolves the user's credential for the tool's platform,
// constructs a per-request client via the platform binding, invokes the tool,
// shapes the response, and returns either a structured success or a typed
// error.
type Server struct {
	registry  *Registry
	creds     CredentialResolver
	shaper    ResponseShaper
	cache     *clientCache
}

// NewServer constructs a Server.
func NewServer(reg *Registry, creds CredentialResolver, shaper ResponseShaper) *Server {
	return &Server{
		registry: reg,
		creds:    creds,
		shaper:   shaper,
		cache:    newClientCache(),
	}
}

// Result is the structured outcome of a single tool call.
type Result struct {
	Tool      string `json:"tool"`
	Platform  string `json:"platform"`
	Output    any    `json:"output,omitempty"`
	Error     *Error `json:"error,omitempty"`
}

// Error is the wire representation of a tool-call failure. Maps from
// mcptool.Error when the handler returns one; otherwise the Code is
// "internal_error".
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// CallTool runs a tool by name for a specific user. Returns a Result with
// Output populated on success or Error populated on failure. The returned
// error is non-nil only for unexpected internal errors (e.g. registry not
// initialised); user-facing errors (unknown tool, missing credential, tool
// failure) are surfaced via Result.Error.
func (s *Server) CallTool(ctx context.Context, userID, name string, input json.RawMessage) Result {
	platform, tool, ok := s.registry.Lookup(name)
	if !ok {
		return Result{Tool: name, Error: &Error{Code: "unknown_tool", Message: fmt.Sprintf("no tool named %q", name)}}
	}
	res := Result{Tool: name, Platform: platform}

	binding, ok := s.registry.Binding(platform)
	if !ok {
		res.Error = &Error{Code: "internal_error", Message: "registry inconsistency: tool has no platform binding"}
		return res
	}
	client, err := s.resolveClient(ctx, userID, binding)
	if err != nil {
		res.Error = mapErr(err)
		return res
	}

	out, err := tool.Invoke(ctx, client, input)
	if err != nil {
		res.Error = mapErr(err)
		return res
	}
	res.Output = s.shaper.Shape(out)
	return res
}

// resolveClient looks up the per-request client for (user, platform), using
// the per-request client cache to avoid creating multiple clients in the same
// request.
//
// Bindings flagged with NoCredentials skip the credentials lookup and are
// invoked with a nil credential blob; the authenticated user ID is always
// available to the NewClient callback via mcp.UserIDFromContext(ctx).
func (s *Server) resolveClient(ctx context.Context, userID string, binding PlatformBinding) (any, error) {
	if binding.NewClient == nil {
		return nil, &mcptool.Error{
			Code:    "binding_misconfigured",
			Message: fmt.Sprintf("platform %s has no NewClient factory", binding.Platform()),
		}
	}
	key := userID + "\x00" + binding.Platform()
	if c, ok := s.cache.get(ctx, key); ok {
		return c, nil
	}
	ctx = withUserID(ctx, userID)

	var credBlob json.RawMessage
	if !binding.NoCredentials {
		blob, err := s.creds.Decrypted(ctx, userID, binding.Platform())
		if err != nil {
			if errors.Is(err, credentials.ErrNotFound) {
				return nil, &mcptool.Error{
					Code:    "credential_missing",
					Message: fmt.Sprintf("no %s credential connected for this user — connect it in Settings", binding.Platform()),
					Data:    map[string]any{"platform": binding.Platform()},
				}
			}
			return nil, &mcptool.Error{
				Code:    "credential_unreadable",
				Message: fmt.Sprintf("could not decrypt %s credential: %v", binding.Platform(), err),
			}
		}
		credBlob = blob
	}

	c, err := binding.NewClient(ctx, credBlob)
	if err != nil {
		return nil, &mcptool.Error{
			Code:    "credential_invalid",
			Message: fmt.Sprintf("invalid %s credential: %v", binding.Platform(), err),
			Data:    map[string]any{"platform": binding.Platform()},
		}
	}
	s.cache.put(ctx, key, c)
	return c, nil
}

// ListTools returns descriptors for every registered tool. Used by the
// JSON-RPC `tools/list` method and by the inventory generator.
func (s *Server) ListTools() []ToolDescriptor {
	tools := s.registry.Tools()
	out := make([]ToolDescriptor, len(tools))
	for i, t := range tools {
		platform, _, _ := s.registry.Lookup(t.Name)
		out[i] = ToolDescriptor{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Platform:    platform,
			WrapsMethod: t.WrapsMethod,
			Tags:        t.Tags,
		}
	}
	return out
}

// ToolDescriptor is the public, marshalable view of a Tool. Suitable for
// `tools/list` JSON-RPC responses and inventory generation.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Platform    string         `json:"platform"`
	WrapsMethod string         `json:"wraps_method,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
}

// mapErr converts an arbitrary error returned by Tool.Invoke into the
// structured Error wire shape.
func mapErr(err error) *Error {
	var me *mcptool.Error
	if errors.As(err, &me) {
		return &Error{
			Code:      me.Code,
			Message:   me.Message,
			Retryable: me.Retryable,
			Data:      me.Data,
		}
	}
	return &Error{Code: "internal_error", Message: err.Error()}
}
