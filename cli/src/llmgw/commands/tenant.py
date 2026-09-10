import click
from rich import print as rprint
from rich.table import Table

from ..client import GatewayClient
from ..utils import confirm_destructive, resolve_id


@click.group()
def tenant():
    """Manage B2B Tenants"""

@tenant.command(name="list")
@click.pass_obj
def tenant_list(client: GatewayClient):
    """List all tenants"""
    resp = client.request("GET", "/admin/tenants")
    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    if not data:
        rprint("[yellow]No tenants found.[/yellow]")
        return

    table = Table(title="Tenants")
    table.add_column("ID", style="cyan")
    table.add_column("NAME", style="white")
    table.add_column("TIER", style="yellow")
    table.add_column("BALANCE", style="green")

    for t in data:
        table.add_row(
            str(t.get("id", "")),
            str(t.get("name", "")),
            str(t.get("tier", "")),
            f"${t.get('credit_balance', 0.0):.2f}"
        )
    rprint(table)

@tenant.command(name="create")
@click.option("--name", required=True, help="Tenant name")
@click.option("--tier", default="free", help="Tenant tier (free, starter, pro, enterprise)")
@click.pass_obj
def tenant_create(client: GatewayClient, name: str, tier: str):
    """Create a new tenant"""
    payload = {"name": name, "tier": tier}
    resp = client.request("POST", "/admin/tenants", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Tenant Created:[/bold green]")
    rprint(resp.json())

@tenant.command(name="update")
@click.option("--id", required=True, help="Tenant ID or Name")
@click.option("--name", help="New Tenant name")
@click.option("--tier", help="New Tenant tier")
@click.pass_obj
def tenant_update(client: GatewayClient, id: str, name: str, tier: str):
    """Update a tenant"""
    tenant_id = resolve_id(
        client, id,
        fetch=lambda: client.request("GET", "/admin/tenants").json(),
        name_key="name", what="tenant"
    )
    payload = {}
    if name:
        payload["name"] = name
    if tier:
        payload["tier"] = tier

    resp = client.request("PATCH", f"/admin/tenants/{tenant_id}", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Tenant Updated:[/bold green]")
    rprint(resp.json())

@tenant.command(name="delete")
@click.option("--id", required=True, help="Tenant ID or Name")
@click.option("--yes", is_flag=True, help="Skip confirmation prompt")
@click.pass_obj
def tenant_delete(client: GatewayClient, id: str, yes: bool):
    """Delete a tenant"""
    confirm_destructive(client, f"This will permanently delete tenant '{id}'.", yes)
    tenant_id = resolve_id(
        client, id,
        fetch=lambda: client.request("GET", "/admin/tenants").json(),
        name_key="name", what="tenant"
    )
    resp = client.request("DELETE", f"/admin/tenants/{tenant_id}")
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Tenant Deleted.[/bold green]")

@tenant.command(name="topup")
@click.option("--id", required=True, help="Tenant ID or Name")
@click.option("--amount", type=float, required=True, help="Amount in USD to add")
@click.option("--ref", default="", help="Reference (e.g. Stripe Invoice ID)")
@click.pass_obj
def tenant_topup(client: GatewayClient, id: str, amount: float, ref: str):
    """Add credits to a tenant's balance"""
    tenant_id = resolve_id(
        client, id,
        fetch=lambda: client.request("GET", "/admin/tenants").json(),
        name_key="name", what="tenant"
    )
    payload = {"amount": amount, "reference": ref}
    resp = client.request("POST", f"/admin/tenants/{tenant_id}/topup", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint(f"[bold green]✅ Added ${amount:.2f} to tenant balance.[/bold green]")
