from pathlib import Path

import click
from rich import print as rprint

from ..client import GatewayClient


@click.command(name="env-sync")
@click.option("--tenant", required=True, help="Tenant name to ensure exists")
@click.option("--key-name", required=True, help="Name for the project API key")
@click.option("--budget", type=float, default=0.0, help="Budget limit for the key (USD)")
@click.option("--rpm", type=int, default=60, help="Rate limit (RPM) for the key")
@click.option("--public-url", default="http://localhost:8080/v1", help="Public Gateway endpoint to write to .env")
@click.pass_obj
def env_sync(client: GatewayClient, tenant: str, key_name: str, budget: float, rpm: int, public_url: str):
    """Auto-provision a project and write OPENAI_API_BASE/.env config"""
    # 1. Ensure Tenant exists
    resp = client.request("GET", "/admin/tenants")
    tenants = resp.json()
    tenant_obj = next((t for t in tenants if t["name"] == tenant), None)

    if not tenant_obj:
        if not client.json_out:
            rprint(f"Creating missing tenant: {tenant}...")
        resp = client.request("POST", "/admin/tenants", json_data={"name": tenant})
        tenant_obj = resp.json()

    tenant_id = tenant_obj["id"]

    # 2. Ensure Key exists for this tenant
    # Since keys are currently tied to users, we need a user. Let's create a service account user.
    sa_email = f"sa-{tenant}@system.local"
    resp = client.request("GET", "/admin/users")
    users = resp.json()
    user_obj = next((u for u in users if u["email"] == sa_email), None)

    if not user_obj:
        if not client.json_out:
            rprint(f"Creating service account: {sa_email}...")
        resp = client.request("POST", "/admin/users", json_data={
            "email": sa_email, "name": f"Service Account for {tenant}", "role": "user"
        })
        user_obj = resp.json()

        # Add user to tenant? There is no tenant-user mapping API exposed directly yet,
        # but tenant_id might be needed if they are grouped. Assuming global users for now.

    user_id = user_obj["id"]

    # 3. Create a fresh key
    if not client.json_out:
        rprint(f"Provisioning fresh API Key '{key_name}'...")
    resp = client.request("POST", "/admin/keys", json_data={
        "user_id": user_id,
        "name": key_name,
        "budget_limit": budget,
        "rate_limit_rpm": rpm
    })
    key_data = resp.json()
    api_key = key_data["api_key"]

    # 4. Write to .env
    env_path = Path.cwd() / ".env"
    lines = []
    if env_path.exists():
        with open(env_path, "r") as f:
            lines = f.readlines()

    # Remove existing
    lines = [line for line in lines if not line.startswith("OPENAI_API_BASE=") and not line.startswith("OPENAI_API_KEY=")]
    lines.append(f"OPENAI_API_BASE={public_url}\n")
    lines.append(f"OPENAI_API_KEY={api_key}\n")

    with open(env_path, "w") as f:
        f.writelines(lines)

    result = {
        "status": "success",
        "action": "env_synced",
        "tenant_id": tenant_id,
        "user_id": user_id,
        "key_id": key_data["id"],
        "env_file": str(env_path)
    }

    if client.json_out:
        client.emit(result)
    else:
        rprint("[bold green]✅ Environment successfully synced![/bold green]")
        rprint(f"  [cyan]Wrote configuration to:[/cyan] {env_path}")
        rprint(f"  OPENAI_API_BASE={public_url}")
        rprint(f"  OPENAI_API_KEY=sk-...{api_key[-4:]}")
