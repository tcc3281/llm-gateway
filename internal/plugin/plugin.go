package plugin

import (
	"context"
	"net/http"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
)

// HookType identifies the lifecycle execution stage of a plugin.
type HookType string

const (
	HookPreAuth      HookType = "pre_auth"
	HookPostAuth     HookType = "post_auth"
	HookPreRouting   HookType = "pre_routing"
	HookPostUpstream HookType = "post_upstream"
)

// PluginContext holds state for the lifecycle of a single client request.
type PluginContext struct {
	Ctx            context.Context
	APIKey         *model.APIKey
	User           *model.User
	Tenant         *model.Tenant
	CanReq         *canonical.CanonicalRequest
	RequestBody    []byte
	InboundFormat  string
	Model          string
	Provider       string
	ResponseWriter http.ResponseWriter
	Redis          *db.RedisClient
	Postgres       *db.PostgresDB

	// Mutable properties manipulated by plugins:
	ExtraHeaders   map[string]string
	RemoveHeaders  []string
	Stop           bool
	StopStatusCode int
	StopErrorBody  []byte
}

// PreAuthPlugin executes before client API key resolution and authentication.
type PreAuthPlugin interface {
	Name() string
	ExecPreAuth(pctx *PluginContext, cfg map[string]any) error
}

// PostAuthPlugin executes after API key and user/tenant IAM have been resolved.
type PostAuthPlugin interface {
	Name() string
	ExecPostAuth(pctx *PluginContext, cfg map[string]any) error
}

// PreRoutingPlugin executes after permission checks and before dispatching to upstream LLM.
type PreRoutingPlugin interface {
	Name() string
	ExecPreRouting(pctx *PluginContext, cfg map[string]any) error
}

// PostUpstreamPlugin executes after a non-streaming upstream response has been decoded.
type PostUpstreamPlugin interface {
	Name() string
	ExecPostUpstream(pctx *PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error
}

// BasePlugin defines metadata for any plugin implementation.
type BasePlugin interface {
	Name() string
	Hooks() []HookType
}
