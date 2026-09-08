package permission

// Action constants used in IAM policies and permission evaluations.
const (
	// Model actions
	ActionModelUse    = "model:use"
	ActionModelStream = "model:stream"
	ActionModelEmbed  = "model:embed"
	ActionModelList   = "model:list"
	ActionModelImage  = "model:image"
	ActionModelAudio  = "model:audio"

	// Observability & Analytics actions
	ActionAnalyticsRead = "analytics:read"
	ActionLogRead       = "log:read"
	ActionAuditRead     = "audit:read"

	// Key management actions
	ActionKeyCreate = "key:create"
	ActionKeyDelete = "key:delete"
	ActionKeyRotate = "key:rotate"
	ActionKeyRead   = "key:read"

	// User management actions
	ActionUserCreate = "user:create"
	ActionUserUpdate = "user:update"
	ActionUserDelete = "user:delete"
	ActionUserRead   = "user:read"

	// Group management actions
	ActionGroupCreate        = "group:create"
	ActionGroupUpdate        = "group:update"
	ActionGroupDelete        = "group:delete"
	ActionGroupRead          = "group:read"
	ActionGroupManageMembers = "group:manage_members"

	// Provider management actions
	ActionProviderRead   = "provider:read"
	ActionProviderUpdate = "provider:update"

	// Virtual model actions
	ActionVirtualModelRead   = "virtual_model:read"
	ActionVirtualModelCreate = "virtual_model:create"
	ActionVirtualModelUpdate = "virtual_model:update"
	ActionVirtualModelDelete = "virtual_model:delete"
)

// ActionDef represents an IAM action metadata item for frontend policy builders.
type ActionDef struct {
	Action      string `json:"action"`
	Namespace   string `json:"namespace"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AllActions is the comprehensive catalog of all actions supported by the gateway.
var AllActions = []ActionDef{
	// Model inference
	{Action: ActionModelUse, Namespace: "model", Label: "Chat / Completion", Description: "Generate completions via chat or prompt"},
	{Action: ActionModelStream, Namespace: "model", Label: "Streaming Response", Description: "Receive responses as SSE streams"},
	{Action: ActionModelEmbed, Namespace: "model", Label: "Embeddings", Description: "Generate vector embeddings from text"},
	{Action: ActionModelList, Namespace: "model", Label: "List Models", Description: "View catalog and available model IDs"},
	{Action: ActionModelImage, Namespace: "model", Label: "Image Generation", Description: "DALL-E / Imagen multimodal generations"},
	{Action: ActionModelAudio, Namespace: "model", Label: "Audio & Transcription", Description: "Whisper audio transcription and TTS"},

	// Observability
	{Action: ActionAnalyticsRead, Namespace: "analytics", Label: "View Analytics", Description: "Inspect dashboards, spending, and usage charts"},
	{Action: ActionLogRead, Namespace: "log", Label: "View Usage Logs", Description: "Browse request logs, latency, and structural shapes"},
	{Action: ActionAuditRead, Namespace: "audit", Label: "View Audit Trail", Description: "Inspect admin actions and security event logs"},

	// Key Management
	{Action: ActionKeyCreate, Namespace: "key", Label: "Create API Keys", Description: "Issue new scoped virtual API keys"},
	{Action: ActionKeyRead, Namespace: "key", Label: "View API Keys", Description: "List and view metadata of issued API keys"},
	{Action: ActionKeyDelete, Namespace: "key", Label: "Revoke API Keys", Description: "Deactivate or revoke active API keys"},
	{Action: ActionKeyRotate, Namespace: "key", Label: "Rotate API Keys", Description: "Cycle keys and refresh credentials"},

	// User Management
	{Action: ActionUserCreate, Namespace: "user", Label: "Create Users", Description: "Add new team members or service users"},
	{Action: ActionUserRead, Namespace: "user", Label: "View Users", Description: "List registered users and group associations"},
	{Action: ActionUserUpdate, Namespace: "user", Label: "Update Users", Description: "Modify roles and profile details"},
	{Action: ActionUserDelete, Namespace: "user", Label: "Delete Users", Description: "Remove user accounts from the gateway"},

	// Group & IAM Management
	{Action: ActionGroupCreate, Namespace: "group", Label: "Create Groups", Description: "Create new departmental IAM groups"},
	{Action: ActionGroupRead, Namespace: "group", Label: "View Groups", Description: "Inspect IAM groups, budgets, and policies"},
	{Action: ActionGroupUpdate, Namespace: "group", Label: "Update Groups", Description: "Change group budgets, RPM limits, or settings"},
	{Action: ActionGroupDelete, Namespace: "group", Label: "Delete Groups", Description: "Remove IAM groups and detached policies"},
	{Action: ActionGroupManageMembers, Namespace: "group", Label: "Manage Group Members", Description: "Assign or remove members within the group"},

	// Provider Management
	{Action: ActionProviderRead, Namespace: "provider", Label: "View Providers", Description: "Inspect configured upstream AI providers"},
	{Action: ActionProviderUpdate, Namespace: "provider", Label: "Configure Providers", Description: "Update provider API keys and routing configuration"},

	// Virtual Models & Routing
	{Action: ActionVirtualModelRead, Namespace: "virtual_model", Label: "View Virtual Models", Description: "List and inspect virtual model aliases and routing rules"},
	{Action: ActionVirtualModelCreate, Namespace: "virtual_model", Label: "Create Virtual Models", Description: "Define new virtual models with dynamic targets and canary weights"},
	{Action: ActionVirtualModelUpdate, Namespace: "virtual_model", Label: "Update Virtual Models", Description: "Modify virtual model routing strategies, weights, and fallbacks"},
	{Action: ActionVirtualModelDelete, Namespace: "virtual_model", Label: "Delete Virtual Models", Description: "Remove virtual model aliases"},
}
