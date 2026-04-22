package notificationsmcp

import (
	"context"

	"github.com/teslashibe/mcptool"
)

// SearchInput is the typed input for notifications_search.
type SearchInput struct {
	Query      string `json:"query" jsonschema:"description=Full-text query against title + content (Postgres plainto_tsquery),required"`
	Since      string `json:"since,omitempty" jsonschema:"description=RFC3339 lower bound on captured_at"`
	Until      string `json:"until,omitempty" jsonschema:"description=RFC3339 upper bound on captured_at"`
	AppPackage string `json:"app_package,omitempty" jsonschema:"description=Restrict to a single app package id"`
	Limit      int    `json:"limit,omitempty" jsonschema:"description=cap on returned events,minimum=1,maximum=200,default=50"`
}

func runSearch(ctx context.Context, c *Client, in SearchInput) (any, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	opts, err := buildListOpts(in.Since, in.Until, in.AppPackage, in.Limit)
	if err != nil {
		return nil, err
	}
	return c.Svc.Search(ctx, c.UserID, in.Query, opts)
}

var searchTools = []mcptool.Tool{
	mcptool.Define[*Client, SearchInput](
		"notifications_search",
		"Full-text search across notification titles and bodies (e.g. 'Sarah Sunset listing')",
		"Search",
		runSearch,
	),
}
