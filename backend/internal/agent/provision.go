package agent

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teslashibe/agent-setup/backend/internal/config"
)

// UserAgent is the cached pair of Anthropic Agent + Environment IDs that
// belong to a single user. The IDs are persisted in the users table by the
// Provisioner so we don't re-create them on every chat.
type UserAgent struct {
	AgentID       string
	EnvironmentID string
	ProvisionedAt time.Time
}

// MCPEndpointFn returns the MCP server URL the user's Agent should connect
// to. The Anthropic SDK's URLMCPServer config does not accept custom
// authorization headers, so the per-user JWT is encoded in the URL itself
// (e.g. /mcp/v1/u/<jwt>) and validated by the MCP transport.
type MCPEndpointFn func(ctx context.Context, userID string) (mcpURL string, err error)

// Provisioner lazily creates per-user Anthropic Agent + Environment
// resources, caches them in the users table, and returns the cached pair on
// subsequent calls.
//
// The Provisioner is intentionally separate from agent.Service so the older,
// shared-agent flow (cmd/provision/main.go) keeps working unchanged. Wire it
// in main.go and call EnsureForUser before any code that previously read
// cfg.AnthropicAgentID.
type Provisioner struct {
	cfg      config.Config
	client   anthropic.Client
	pool     *pgxpool.Pool
	endpoint MCPEndpointFn
	model    anthropic.BetaManagedAgentsModel
	system   string

	mu       sync.Mutex
	inflight map[string]*sync.Mutex
}

// ProvisionerOptions tweaks Provisioner defaults at construction time.
type ProvisionerOptions struct {
	// Model overrides the Claude model used for per-user agents. Defaults
	// to claude-sonnet-4-5 when zero.
	Model anthropic.BetaManagedAgentsModel
	// SystemPrompt overrides the agent's system prompt.
	SystemPrompt string
}

// NewProvisioner constructs a Provisioner. endpoint is required.
func NewProvisioner(cfg config.Config, client anthropic.Client, pool *pgxpool.Pool, endpoint MCPEndpointFn, opts ProvisionerOptions) (*Provisioner, error) {
	if endpoint == nil {
		return nil, errors.New("agent: MCP endpoint factory is required")
	}
	model := opts.Model
	if model == "" {
		model = anthropic.BetaManagedAgentsModelClaudeSonnet4_5
	}
	system := opts.SystemPrompt
	if system == "" {
		system = defaultSystemPrompt
	}
	return &Provisioner{
		cfg:      cfg,
		client:   client,
		pool:     pool,
		endpoint: endpoint,
		model:    model,
		system:   system,
		inflight: map[string]*sync.Mutex{},
	}, nil
}

const defaultSystemPrompt = `You are an autonomous engagement agent for a single human operator.

Available tools come from a unified MCP server that wraps the user's connected social platforms (LinkedIn, X, Reddit, Hacker News, Facebook, Instagram, TikTok, ProductHunt, Threads, Nextdoor, ElevenLabs, Codegen). Tool names are prefixed with their platform (e.g. linkedin_search_people).

When a tool requires platform credentials that the user has not yet connected, the call returns an MCP error with code "credential_missing". When you receive that error, surface a short message asking the user to open Settings → Connections and add the relevant cookie. Do not retry until the user confirms connection.

Prefer fine-grained tool calls and small page sizes. Always summarise tool output before presenting it to the user.`

// EnsureForUser returns the cached UserAgent for userID, provisioning a new
// pair if missing. Concurrent calls for the same user are serialised so we
// only ever create one Agent per user.
func (p *Provisioner) EnsureForUser(ctx context.Context, userID string) (UserAgent, error) {
	if cached, ok, err := p.lookup(ctx, userID); err != nil {
		return UserAgent{}, err
	} else if ok {
		return cached, nil
	}

	lock := p.lockFor(userID)
	lock.Lock()
	defer lock.Unlock()

	if cached, ok, err := p.lookup(ctx, userID); err != nil {
		return UserAgent{}, err
	} else if ok {
		return cached, nil
	}

	envID, err := p.createEnvironment(ctx, userID)
	if err != nil {
		return UserAgent{}, fmt.Errorf("create environment: %w", err)
	}
	agentID, err := p.createAgent(ctx, userID)
	if err != nil {
		return UserAgent{}, fmt.Errorf("create agent: %w", err)
	}
	now := time.Now().UTC()
	if _, err := p.pool.Exec(ctx, `
		UPDATE users
		   SET anthropic_agent_id       = $1,
		       anthropic_environment_id = $2,
		       anthropic_provisioned_at = $3
		 WHERE id = $4`, agentID, envID, now, userID); err != nil {
		return UserAgent{}, fmt.Errorf("persist provisioned IDs: %w", err)
	}
	return UserAgent{AgentID: agentID, EnvironmentID: envID, ProvisionedAt: now}, nil
}

func (p *Provisioner) lookup(ctx context.Context, userID string) (UserAgent, bool, error) {
	var (
		ua   UserAgent
		aID  *string
		eID  *string
		when *time.Time
	)
	err := p.pool.QueryRow(ctx, `
		SELECT anthropic_agent_id, anthropic_environment_id, anthropic_provisioned_at
		  FROM users WHERE id = $1`, userID).Scan(&aID, &eID, &when)
	if errors.Is(err, pgx.ErrNoRows) {
		return ua, false, fmt.Errorf("user %s not found", userID)
	}
	if err != nil {
		return ua, false, err
	}
	if aID != nil && *aID != "" && eID != nil && *eID != "" {
		ua.AgentID = *aID
		ua.EnvironmentID = *eID
		if when != nil {
			ua.ProvisionedAt = *when
		}
		return ua, true, nil
	}
	return ua, false, nil
}

func (p *Provisioner) lockFor(userID string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l, ok := p.inflight[userID]; ok {
		return l
	}
	l := &sync.Mutex{}
	p.inflight[userID] = l
	return l
}

// createEnvironment provisions a sandbox Environment for this user. The
// Environment is shaped permissively (unrestricted networking) so MCP calls
// to the user's per-user MCP URL can complete; this matches the original
// shared-agent flow in cmd/provision/main.go.
func (p *Provisioner) createEnvironment(ctx context.Context, userID string) (string, error) {
	env, err := p.client.Beta.Environments.New(ctx, anthropic.BetaEnvironmentNewParams{
		Name: fmt.Sprintf("agent-setup-env-%s", userID),
		Config: anthropic.BetaCloudConfigParams{
			Networking: anthropic.BetaCloudConfigParamsNetworkingUnion{
				OfUnrestricted: &anthropic.BetaUnrestrictedNetworkParam{},
			},
		},
	})
	if err != nil {
		return "", err
	}
	return env.ID, nil
}

// createAgent provisions an Anthropic Agent bound to the per-user MCP server
// URL. The MCP toolset name "engagement" must match the URLMCPServer name so
// the agent can route tool calls to the correct server.
func (p *Provisioner) createAgent(ctx context.Context, userID string) (string, error) {
	mcpURL, err := p.endpoint(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("mcp endpoint: %w", err)
	}
	if _, err := url.Parse(mcpURL); err != nil {
		return "", fmt.Errorf("invalid MCP url %q: %w", mcpURL, err)
	}
	const mcpServerName = "engagement"

	a, err := p.client.Beta.Agents.New(ctx, anthropic.BetaAgentNewParams{
		Name:        fmt.Sprintf("agent-setup-user-%s", userID),
		Description: anthropic.String("Per-user engagement agent with platform MCP tools"),
		System:      anthropic.String(p.system),
		Model: anthropic.BetaManagedAgentsModelConfigParams{
			ID: p.model,
		},
		MCPServers: []anthropic.BetaManagedAgentsURLMCPServerParams{{
			Name: mcpServerName,
			Type: anthropic.BetaManagedAgentsURLMCPServerParamsTypeURL,
			URL:  mcpURL,
		}},
		Tools: []anthropic.BetaAgentNewParamsToolUnion{
			{OfMCPToolset: &anthropic.BetaManagedAgentsMCPToolsetParams{
				MCPServerName: mcpServerName,
				Type:          anthropic.BetaManagedAgentsMCPToolsetParamsTypeMCPToolset,
			}},
		},
	})
	if err != nil {
		return "", err
	}
	return a.ID, nil
}
