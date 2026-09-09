import json

import click
from fastmcp import Context, FastMCP

from ..client import GatewayClient


@click.command(name="mcp-serve")
@click.pass_obj
def mcp_serve(client: GatewayClient):
    """Run MCP server to expose llmgw functionality natively to IDE Agents"""
    # FastMCP uses the client internally
    mcp = FastMCP("llmgw")
    
    @mcp.tool()
    def get_tenants() -> list[dict]:
        """List all active B2B tenants"""
        return client.request("GET", "/admin/tenants").json()
        
    @mcp.tool()
    def create_tenant(name: str, tier: str = "free") -> dict:
        """Create a new tenant with a given name and tier (free, starter, pro, enterprise)"""
        return client.request("POST", "/admin/tenants", json_data={"name": name, "tier": tier}).json()

    @mcp.tool()
    async def topup_tenant(tenant_name: str, amount_usd: float, ctx: Context) -> dict:
        """Add USD credits to a tenant. Requires user approval."""
        result = await ctx.elicit(f"Approve topping up ${amount_usd} to tenant '{tenant_name}'?", response_type=bool)
        if result.action != "accept" or not result.data:
            return {"error": "Action rejected by user."}

        tenants = get_tenants()
        tenant = next((t for t in tenants if t["name"] == tenant_name), None)
        if not tenant:
            return {"error": f"Tenant {tenant_name} not found"}
        return client.request("POST", f"/admin/tenants/{tenant['id']}/topup", json_data={"amount": amount_usd}).json()

    @mcp.tool()
    def get_plugins() -> list[dict]:
        """List all available plugins (Guardrails, Rate Limits, etc.)"""
        return client.request("GET", "/admin/plugins").json()
        
    @mcp.tool()
    async def bind_plugin(plugin_name: str, scope_level: str, config: str, priority: int = 100, *, ctx: Context) -> dict:
        """Bind a plugin to a scope level (global, tenant). Config must be a valid JSON string. Requires user approval."""
        result = await ctx.elicit(f"Approve binding plugin '{plugin_name}' to scope '{scope_level}' with priority {priority}?", response_type=bool)
        if result.action != "accept" or not result.data:
            return {"error": "Action rejected by user."}

        payload = {
            "plugin_name": plugin_name,
            "scope_level": scope_level,
            "config": json.loads(config),
            "priority": priority,
            "is_enabled": True
        }
        return client.request("POST", "/admin/plugin-bindings", json_data=payload).json()

    @mcp.tool()
    def provision_project_env(tenant_name: str, key_name: str, budget: float = 0.0) -> str:
        """Automated macro to create a tenant, a service account, a new API key, and return the OPENAI_API_KEY value."""
        # Using the same logic as env_sync
        from .env_sync import env_sync
        ctx = click.Context(env_sync)
        # It's better to implement inline here to return string directly
        tenants = get_tenants()
        tenant = next((t for t in tenants if t["name"] == tenant_name), None)
        if not tenant:
            tenant = create_tenant(tenant_name)
        
        sa_email = f"sa-{tenant_name}@system.local"
        users = client.request("GET", "/admin/users").json()
        user = next((u for u in users if u["email"] == sa_email), None)
        if not user:
            user = client.request("POST", "/admin/users", json_data={
                "email": sa_email, "name": f"Service Account {tenant_name}", "role": "user"
            }).json()
            
        key_data = client.request("POST", "/admin/keys", json_data={
            "user_id": user["id"],
            "name": key_name,
            "budget_limit": budget,
            "rate_limit_rpm": 60
        }).json()
        
        return f"Successfully provisioned! Use OPENAI_API_BASE=http://localhost:8080/v1 and OPENAI_API_KEY={key_data['api_key']}"

    # Run the MCP server via stdio
    mcp.run()
