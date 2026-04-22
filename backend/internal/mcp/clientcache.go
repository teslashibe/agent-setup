package mcp

import (
	"context"
	"sync"
)

type ctxKey int

const cacheKey ctxKey = 0

// clientCache lets a single HTTP request reuse the constructed *Client
// across multiple tool invocations for the same (user, platform). Without
// it, an MCP request that hits e.g. linkedin_search_people followed by
// linkedin_get_profile would build two separate *linkedin.Client instances.
//
// The cache is implicitly per-request because the cache itself is stored in
// the request context (see WithRequest).
type clientCache struct{}

func newClientCache() *clientCache { return &clientCache{} }

type entry struct {
	mu sync.Mutex
	v  any
}

type cacheMap struct {
	mu sync.Mutex
	m  map[string]*entry
}

// WithRequest scopes a per-request cache to ctx so that resolveClient can
// reuse clients across multiple tool calls in the same logical request.
func WithRequest(ctx context.Context) context.Context {
	if v, _ := ctx.Value(cacheKey).(*cacheMap); v != nil {
		return ctx
	}
	return context.WithValue(ctx, cacheKey, &cacheMap{m: map[string]*entry{}})
}

func (c *clientCache) get(ctx context.Context, key string) (any, bool) {
	cm, _ := ctx.Value(cacheKey).(*cacheMap)
	if cm == nil {
		return nil, false
	}
	cm.mu.Lock()
	e, ok := cm.m[key]
	cm.mu.Unlock()
	if !ok {
		return nil, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.v == nil {
		return nil, false
	}
	return e.v, true
}

func (c *clientCache) put(ctx context.Context, key string, v any) {
	cm, _ := ctx.Value(cacheKey).(*cacheMap)
	if cm == nil {
		return
	}
	cm.mu.Lock()
	e, ok := cm.m[key]
	if !ok {
		e = &entry{}
		cm.m[key] = e
	}
	cm.mu.Unlock()
	e.mu.Lock()
	e.v = v
	e.mu.Unlock()
}
