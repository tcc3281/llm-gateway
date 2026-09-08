# LLM Gateway - IAM-Style Permission System Plan

> Mục tiêu: Thay thế hệ thống role cố định bằng hệ thống quyền linh hoạt kiểu AWS IAM
> Root admin → Groups với policies động → Users thuộc groups → API keys với restrictions

---

## Kiến trúc tổng quan

```
                    ┌─────────────────────────────┐
                    │        Root Admin            │
                    │  (toàn quyền, set up đầu tiên)│
                    └──────────┬──────────────────┘
                               │ tạo
                    ┌──────────▼──────────────────┐
                    │         Groups               │
                    │  (phòng ban / team / role)    │
                    │  ┌─────────────────────┐     │
                    │  │ Policies (JSON)      │     │
                    │  │ - action:*           │     │
                    │  │ - resource:*         │     │
                    │  │ - effect: allow/deny │     │
                    │  └─────────────────────┘     │
                    └──────────┬──────────────────┘
                               │ members
                    ┌──────────▼──────────────────┐
                    │         Users                │
                    │  (thuộc 1 hoặc nhiều groups)  │
                    └──────────┬──────────────────┘
                               │ owns
                    ┌──────────▼──────────────────┐
                    │       API Keys               │
                    │  (budget, rate limit,         │
                    │   model restrictions)         │
                    └─────────────────────────────┘
```

---

## Permission Model

### Actions (verbs)

```
# Model management
model:list          # GET /v1/models, /admin/models
model:read          # GET /admin/models/catalog
model:create        # POST /admin/models (upsert pricing)
model:update        # PATCH /admin/models
model:delete        # DELETE /admin/models
model:sync          # POST /admin/models/sync (from LiteLLM)
model:discover      # POST /admin/models/discover

# Model usage (gateway)
model:use           # Gọi model qua gateway (chat completions, embeddings)

# Key management
key:list            # GET /admin/keys
key:create          # POST /admin/keys
key:update          # PATCH /admin/keys
key:revoke          # DELETE /admin/keys
key:reactivate      # POST /admin/keys/reactivate

# User management
user:list           # GET /admin/users
user:create         # POST /admin/users
user:update         # PATCH /admin/users
user:delete         # DELETE /admin/users

# Provider management
provider:list       # GET /admin/providers
provider:create     # POST /admin/providers (set key)
provider:update     # POST /admin/providers (update key)
provider:delete     # DELETE /admin/providers

# Analytics & logs
analytics:read      # GET /admin/analytics/*
log:read            # GET /admin/logs
log:detail          # GET /admin/logs/{id}

# System
system:read         # GET /admin/system/info
group:list          # GET /admin/groups
group:create        # POST /admin/groups
group:update        # PATCH /admin/groups
group:delete        # DELETE /admin/groups
group:assign        # POST /admin/groups/assign (add user to group)
group:remove        # POST /admin/groups/remove (remove user from group)

# Audit
audit:read          # GET /admin/audit-logs
```

### Resources (nouns)

```
model:*                    # Tất cả models
model:gpt-4o               # Model cụ thể
model:claude-*             # Pattern matching (wildcard)

key:*                      # Tất cả keys
key:<uuid>                 # Key cụ thể

user:*                     # Tất cả users
user:<uuid>                # User cụ thể

provider:*                 # Tất cả providers
provider:openai            # Provider cụ thể

group:*                    # Tất cả groups
group:<uuid>               # Group cụ thể

analytics:*                # Tất cả analytics
log:*                      # Tất cả logs
system:*                   # System info
```

### Policy Document (JSON format, tương tự AWS IAM)

```json
{
  "Version": "1",
  "Statement": [
    {
      "Sid": "AllowModelUsage",
      "Effect": "Allow",
      "Action": ["model:use"],
      "Resource": ["model:gpt-4o", "model:claude-3.5-sonnet", "model:gemini-*"]
    },
    {
      "Sid": "DenyExpensiveModels",
      "Effect": "Deny",
      "Action": ["model:use"],
      "Resource": ["model:gpt-4o-128k", "model:claude-3-opus"]
    },
    {
      "Sid": "AllowKeyManagement",
      "Effect": "Allow",
      "Action": ["key:list", "key:create", "key:update"],
      "Resource": ["key:*"]
    }
  ]
}
```

**Evaluation logic**: Deny总是 wins (explicit deny > allow). Nếu không có matching policy → implicit deny.

---

## Database Schema

### Bảng mới

```sql
-- 1. Groups (phòng ban / teams)
CREATE TABLE IF NOT EXISTS groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    budget_limit_monthly NUMERIC(12,6) DEFAULT 0,  -- 0 = unlimited
    rate_limit_rpm INT DEFAULT 0,                   -- 0 = use key's own limit
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. Group Policies (IAM-style policy documents)
CREATE TABLE IF NOT EXISTS group_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,               -- human-readable policy name
    policy JSONB NOT NULL,                     -- IAM-style policy document
    priority INT DEFAULT 0,                    -- higher = evaluated first
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(group_id, name)
);

-- 3. User-Group mapping (user có thể thuộc nhiều groups)
CREATE TABLE IF NOT EXISTS user_groups (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);

-- 4. Key-level model restrictions (optional, per-key overrides)
CREATE TABLE IF NOT EXISTS key_model_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    model_pattern VARCHAR(200) NOT NULL,       -- "gpt-4o" or "claude-*"
    is_allowlist BOOLEAN DEFAULT TRUE,         -- TRUE = whitelist, FALSE = blacklist
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(key_id, model_pattern)
);

-- 5. Audit log
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id UUID,                             -- user_id hoặc admin key identifier
    actor_type VARCHAR(20) NOT NULL,           -- 'root', 'admin', 'user', 'system'
    actor_ip INET,
    action VARCHAR(50) NOT NULL,               -- 'key.create', 'user.delete'
    target_type VARCHAR(50),                   -- 'user', 'key', 'group', 'provider'
    target_id VARCHAR(100),
    details JSONB,                             -- request body diff, old/new values
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_actor ON audit_logs (actor_id, created_at DESC);
CREATE INDEX idx_audit_logs_action ON audit_logs (action, created_at DESC);
CREATE INDEX idx_audit_logs_target ON audit_logs (target_type, target_id, created_at DESC);

-- 6. Budget periods cho API keys
ALTER TABLE api_keys ADD COLUMN budget_period VARCHAR(20) DEFAULT NULL;
    -- NULL = lifetime (current behavior), 'daily', 'monthly', 'weekly'
ALTER TABLE api_keys ADD COLUMN budget_period_reset_at TIMESTAMPTZ DEFAULT NULL;
```

### Bảng hiện có (cần sửa)

```sql
-- users: thêm is_root boolean
ALTER TABLE users ADD COLUMN is_root BOOLEAN DEFAULT FALSE;
    -- Root admin bypass tất cả permission checks
```

---

## Implementation Plan - 7 Phases

### Phase 0: Quick Security Fixes (1-2h)
> Fix các vulnerability hiện tại trước khi build thêm

**Files affected**: `auth/auth.go`, `config/config.go`, `handler/admin.go`

1. **Fix timing attack** - `auth/auth.go`
   ```go
   // Trước (VULNERABLE)
   func (s *AuthService) ValidateAdminKey(key string) bool {
       return key != "" && key == s.adminKey
   }
   
   // Sau (SAFE)
   import "crypto/subtle"
   func (s *AuthService) ValidateAdminKey(key string) bool {
       if key == "" || s.adminKey == "" {
           return false
       }
       return subtle.ConstantTimeCompare([]byte(key), []byte(s.adminKey)) == 1
   }
   ```

2. **Startup warning cho default admin key** - `config/config.go`
   ```go
   func (c *Config) Validate() error {
       if c.AdminAPIKey == "sk-admin-secret-key-change-me" {
           log.Println("⚠️  WARNING: Using default ADMIN_API_KEY. Set ADMIN_API_KEY env var!")
       }
       if c.ProviderMasterKey == "" {
           log.Println("⚠️  WARNING: Provider keys stored in plaintext. Set PROVIDER_MASTER_KEY!")
       }
       return nil
   }
   ```

3. **Add audit log table + basic logging** - migration + `handler/admin.go`
   - Tạo bảng `audit_logs`
   - Thêm `logAudit()` helper function
   - Gọi `logAudit()` ở mọi admin mutation endpoint (create/update/delete)

4. **Admin IP whitelist硬化** - `handler/admin.go`
   - Thêm check: nếu `ADMIN_ALLOWED_IPS` empty → reject connection (hiện tại đang allow all)
   - Document rằng phải dùng trusted proxy

---

### Phase 1: Core Permission Engine (4-6h)
> Xây permission evaluation engine - heart của hệ thống

**Files mới**: `internal/permission/permission.go`, `internal/permission/policy.go`

```go
// internal/permission/permission.go

type PermissionEngine struct {
    pg *db.PostgresDB
    // Cache policies in memory, refresh every 60s
    mu       sync.RWMutex
    groupPolicies map[uuid.UUID][]Policy  // group_id -> policies
}

// Check evaluates whether a user has the given action on the given resource.
// Returns (allowed bool, matchedPolicy string, error)
func (pe *PermissionEngine) Check(ctx context.Context, userID uuid.UUID, action, resource string) (bool, string, error) {
    // 1. Check if user is root → always allow
    // 2. Get user's groups
    // 3. Collect all policies from all groups
    // 4. Evaluate: explicit Deny wins over Allow
    // 5. If no matching policy → implicit deny
}

// CheckModelAccess is a convenience wrapper for model:use checks
func (pe *PermissionEngine) CheckModelAccess(ctx context.Context, userID uuid.UUID, modelName string) (bool, error) {
    return pe.Check(ctx, userID, "model:use", "model:"+modelName)
}
```

```go
// internal/permission/policy.go

type Policy struct {
    Version   string
    Statement []Statement
}

type Statement struct {
    Sid      string
    Effect   string   // "Allow" or "Deny"
    Action   []string // ["model:use", "model:list"]
    Resource []string // ["model:gpt-4o", "model:claude-*"]
}

// Evaluate checks if this policy grants/denies the given action+resource
func (p *Policy) Evaluate(action, resource string) (matched bool, effect string) {
    for _, stmt := range p.Statement {
        if actionMatches(stmt.Action, action) && resourceMatches(stmt.Resource, resource) {
            return true, stmt.Effect
        }
    }
    return false, ""
}

// Pattern matching: "claude-*" matches "claude-3.5-sonnet"
func resourceMatches(patterns []string, resource string) bool { ... }
func actionMatches(patterns []string, action string) bool { ... }
```

**Evaluation order**:
1. Root user → allow everything
2. Explicit Deny (from any group policy) → deny
3. Explicit Allow (from any group policy) → allow
4. No match → implicit deny

---

### Phase 2: Database Schema + Migration (2-3h)
> Tạo migration mới cho permission tables

**Files mới**: `migrations/000008_iam_permissions.up.sql`

Migration nội dung như section "Database Schema" ở trên. Sau khi migration:
- Seed 1 root admin user (dùng `is_root = TRUE`)
- Seed 1 default group "Full Access" với policy allow `*:*`

---

### Phase 3: Permission Middleware (3-4h)
> Tích hợp permission engine vào gateway + admin handlers

**Files affected**: `handler/gateway.go`, `handler/admin.go`, `cmd/gateway/main.go`

#### 3a. Gateway Handler - Model Access Control

```go
// handler/gateway.go - authenticate() sau khi validate key

func (h *GatewayHandler) authenticate(r *http.Request) (*model.APIKey, *GateErr) {
    // ... existing key validation + rate limit + budget ...
    
    // NEW: Check model permission (deferred to after model is known)
    // Permission check happens in HandleRequest after model is parsed
    return apiKey, nil
}

// handler/gateway.go - HandleOpenAIChatCompletions
func (h *GatewayHandler) HandleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
    // ... existing auth ...
    
    // NEW: After parsing model from body, check permission
    // parsedModel := extractModel(body)
    // if allowed, _ := h.permEngine.CheckModelAccess(ctx, apiKey.UserID, parsedModel); !allowed {
    //     WriteErrorResponse(w, "openai", 403, "Model not allowed", ...)
    //     return
    // }
}
```

#### 3b. Admin Handler - Endpoint Access Control

```go
// handler/admin.go - checkAdminAuth() mở rộng

func (h *AdminHandler) checkAdminAuth(w http.ResponseWriter, r *http.Request) bool {
    // 1. IP Whitelist (keep existing)
    // 2. Authenticate: admin key OR user session
    // 3. NEW: Extract action from route + method
    //    action := h.routeToAction(r.Method, r.URL.Path)
    //    resource := h.routeToResource(r)
    // 4. NEW: Check permission via engine
    //    if !isRootAdmin {
    //        allowed, _, _ := h.permEngine.Check(ctx, userID, action, resource)
    //        if !allowed { return 403 }
    //    }
    // 5. Audit log
    //    h.logAudit(ctx, actor, action, resource, r)
}

// Route → Action mapping
var routeActions = map[string]map[string]string{
    "GET /admin/models":         {"action": "model:list"},
    "POST /admin/models":        {"action": "model:create"},
    "GET /admin/keys":           {"action": "key:list"},
    "POST /admin/keys":          {"action": "key:create"},
    "DELETE /admin/keys":        {"action": "key:revoke"},
    // ... etc
}
```

---

### Phase 4: Admin API for Groups & Policies (4-5h)
> CRUD endpoints cho managing groups, policies, user-group assignments

**Files mới**: `handler/groups.go`

```
GET    /admin/groups              → List all groups
POST   /admin/groups              → Create group
GET    /admin/groups/{id}         → Get group detail + policies
PATCH  /admin/groups/{id}         → Update group
DELETE /admin/groups/{id}         → Delete group

POST   /admin/groups/{id}/policies     → Add policy to group
DELETE /admin/groups/{id}/policies/{pid} → Remove policy
PATCH  /admin/groups/{id}/policies/{pid} → Update policy

POST   /admin/groups/{id}/members      → Add user to group
DELETE /admin/groups/{id}/members/{uid} → Remove user from group
GET    /admin/groups/{id}/members      → List group members

GET    /admin/audit-logs               → Query audit logs
```

**Admin UI pages mới** (React):
- Groups management page (list/create/edit)
- Policy editor (JSON editor với validation + preview)
- User-group assignment page
- Audit log viewer

---

### Phase 5: Key-Level Model Restrictions (2-3h)
> Per-key model allowlist/denylist cho fine-grained control

**Files affected**: `handler/gateway.go`, `handler/admin.go`

```go
// internal/permission/key_policy.go

func (pe *PermissionEngine) CheckKeyModelAccess(ctx context.Context, keyID uuid.UUID, modelName string) (bool, error) {
    // 1. Query key_model_policies for this key
    // 2. If no policies → allow (default, backward compatible)
    // 3. Evaluate allowlist/denylist
    // 4. Denylist wins over allowlist
}
```

Integration ở `handler/gateway.go`:
- Sau khi authenticate key, trước khi resolve target
- Check both user-level (group policy) AND key-level model restrictions
- Both must pass → allow

---

### Phase 6: Budget Period + User-Level Budget (2-3h)
> Budget theo period + user-level aggregation

**Changes**:
- `api_keys.budget_period`: NULL (lifetime), 'daily', 'weekly', 'monthly'
- `api_keys.budget_period_reset_at`: auto-reset when period expires
- `groups.budget_limit_monthly`: user-level budget ceiling
- Budget check logic:
  ```
  1. Check key budget (with period reset logic)
  2. Check user total budget (sum of all keys)
  3. Check group monthly budget
  4. All must pass → allow
  ```

---

### Phase 7: Polish + Testing (3-4h)
> Integration tests, edge cases, documentation

- Unit tests cho permission engine (policy evaluation, pattern matching)
- Integration tests cho full auth flow
- Edge case: user in multiple groups with conflicting policies
- Edge case: key-level restriction tighter than group policy
- Performance: policy cache hit rate, Redis caching cho permission checks
- Documentation: API docs, admin guide

---

## Timeline Tổng quan

| Phase | Nội dung | Thời gian | Dependencies |
|-------|----------|-----------|-------------|
| 0 | Quick security fixes | 1-2h | - |
| 1 | Permission engine | 4-6h | - |
| 2 | Database migration | 2-3h | Phase 1 |
| 3 | Permission middleware | 3-4h | Phase 1, 2 |
| 4 | Admin API for groups | 4-5h | Phase 2, 3 |
| 5 | Key-level restrictions | 2-3h | Phase 3 |
| 6 | Budget periods | 2-3h | Phase 3 |
| 7 | Polish + testing | 3-4h | Phase 4, 5, 6 |
| **Tổng** | | **21-30h** | |

---

## Migration Path (Backward Compatible)

1. **Phase 0-2**: Không break existing behavior
   - `users.role` vẫn giữ, nhưng permission engine dùng `is_root` + group policies
   - Existing keys tiếp tục hoạt động (no model restriction = allow all)
   - Admin key tiếp tục hoạt động (IP whitelist + admin key)

2. **Phase 3**: Permission checks are ADDITIVE
   - Nếu user có root flag → bypass all checks (backward compatible)
   - Nếu user không có group policy → implicit deny (cần seed default policies)

3. **Phase 4-6**: New features, opt-in
   - Groups là new concept, không affect existing users
   - Budget period là optional, NULL = lifetime (current behavior)

---

## Ví dụ Setup cho công ty

```
Root Admin (is_root = true)
  └── Toàn quyền, set up hệ thống

Group: "AI Engineering Team"
  Policies:
    - Allow: model:use on model:gpt-4o, model:claude-3.5-sonnet, model:gemini-*
    - Allow: model:list, model:read
    - Deny: model:use on model:gpt-4o-128k (too expensive)
  Budget: $500/month
  Members: dev1@company.com, dev2@company.com

Group: "Data Team"  
  Policies:
    - Allow: model:use on model:embeddings-*, model:text-embedding-*
    - Allow: model:list
    - Deny: model:use on model:gpt-*, model:claude-* (no chat models)
  Budget: $200/month
  Members: data1@company.com

Group: "Project Managers"
  Policies:
    - Allow: analytics:read, log:read
    - Allow: key:list (view only, no create)
    - Deny: model:use (no direct model access)
  Members: pm1@company.com

Group: "Team Leads"
  Policies:
    - Allow: key:create, key:update, key:revoke (for their team members only)
    - Allow: analytics:read
    - Allow: model:use on model:gpt-4o-mini (lightweight only)
  Members: lead1@company.com
```

---

## Files tham chiếu

- `docs/AUTH_ASSESSMENT.md` - Đánh giá auth hiện tại
- `internal/auth/auth.go` - Cần fix timing attack
- `internal/handler/gateway.go` - Cần thêm permission checks
- `internal/handler/admin.go` - Cần thêm audit logging + permission checks
- `internal/model/models.go` - Cần mở rộng models
