import click
from rich import print as rprint
from rich.table import Table

from ..client import GatewayClient
from ..utils import confirm_destructive, resolve_id


@click.group()
def group():
    """Manage IAM Groups and RBAC Policies"""

@group.command(name="list")
@click.pass_obj
def group_list(client: GatewayClient):
    """List all IAM groups"""
    resp = client.request("GET", "/admin/groups")
    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    if not data:
        rprint("[yellow]No groups found.[/yellow]")
        return

    table = Table(title="IAM Groups")
    table.add_column("ID", style="cyan")
    table.add_column("TENANT ID", style="magenta")
    table.add_column("NAME", style="white")
    table.add_column("DESCRIPTION", style="yellow")

    for g in data:
        table.add_row(
            str(g.get("id", "")),
            str(g.get("tenant_id", "")),
            str(g.get("name", "")),
            str(g.get("description", ""))
        )
    rprint(table)

@group.command(name="create")
@click.option("--name", required=True, help="Group name")
@click.option("--tenant-id", help="Tenant ID (leave empty for system-wide group)")
@click.option("--desc", help="Group description")
@click.pass_obj
def group_create(client: GatewayClient, name: str, tenant_id: str, desc: str):
    """Create a new IAM group"""
    payload = {"name": name, "tenant_id": tenant_id or None, "description": desc or ""}
    resp = client.request("POST", "/admin/groups", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Group Created:[/bold green]")
    rprint(resp.json())

@group.command(name="delete")
@click.option("--id", required=True, help="Group ID or Name")
@click.option("--yes", is_flag=True, help="Skip confirmation prompt")
@click.pass_obj
def group_delete(client: GatewayClient, id: str, yes: bool):
    """Delete a group"""
    confirm_destructive(client, f"This will permanently delete group '{id}'.", yes)
    group_id = resolve_id(
        client, id,
        fetch=lambda: client.request("GET", "/admin/groups").json(),
        name_key="name", what="group"
    )
    resp = client.request("DELETE", f"/admin/groups/{group_id}")
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Group Deleted.[/bold green]")

@group.command(name="member-add")
@click.option("--group-id", required=True, help="Group ID or Name")
@click.option("--user-id", required=True, help="User ID or Email")
@click.option("--role", default="member", help="Role inside group (admin, member)")
@click.pass_obj
def group_member_add(client: GatewayClient, group_id: str, user_id: str, role: str):
    """Add a user to a group"""
    gid = resolve_id(
        client, group_id,
        fetch=lambda: client.request("GET", "/admin/groups").json(),
        name_key="name", what="group"
    )
    uid = resolve_id(
        client, user_id,
        fetch=lambda: client.request("GET", "/admin/users").json(),
        name_key="email", what="user"
    )
    payload = {"user_id": uid, "role": role}
    resp = client.request("POST", f"/admin/groups/{gid}/members", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint(f"[bold green]✅ User {uid} added to Group {gid}.[/bold green]")
