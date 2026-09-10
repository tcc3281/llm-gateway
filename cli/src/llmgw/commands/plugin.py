import json

import click
from rich import print as rprint
from rich.table import Table

from ..client import GatewayClient
from ..utils import confirm_destructive


@click.group()
def plugin():
    """Manage Plugins and Scope Bindings"""

@plugin.command(name="list")
@click.pass_obj
def plugin_list(client: GatewayClient):
    """List all available plugins in the catalog"""
    resp = client.request("GET", "/admin/plugins")
    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    if not data:
        rprint("[yellow]No plugins found.[/yellow]")
        return

    table = Table(title="Plugin Catalog")
    table.add_column("NAME", style="cyan")
    table.add_column("DESCRIPTION", style="white")
    table.add_column("VERSION", style="yellow")

    for p in data:
        table.add_row(
            str(p.get("name", "")),
            str(p.get("description", "")),
            str(p.get("version", ""))
        )
    rprint(table)

@plugin.command(name="bindings")
@click.option("--tenant-id", help="Filter bindings by tenant")
@click.option("--plugin-name", help="Filter bindings by plugin name")
@click.pass_obj
def plugin_bindings(client: GatewayClient, tenant_id: str, plugin_name: str):
    """List active plugin bindings"""
    params = {}
    if tenant_id:
        params["tenant_id"] = tenant_id
    if plugin_name:
        params["plugin_name"] = plugin_name

    resp = client.request("GET", "/admin/plugin-bindings", params=params or None)
    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    if not data:
        rprint("[yellow]No plugin bindings found.[/yellow]")
        return

    table = Table(title="Active Plugin Bindings")
    table.add_column("ID", style="cyan")
    table.add_column("PLUGIN", style="magenta")
    table.add_column("SCOPE LEVEL", style="yellow")
    table.add_column("PRIORITY", style="green")
    table.add_column("ENABLED", style="blue")

    for b in data:
        table.add_row(
            str(b.get("id", "")),
            str(b.get("plugin_name", "")),
            str(b.get("scope_level", "")),
            str(b.get("priority", 0)),
            str(b.get("is_enabled", False))
        )
    rprint(table)

@plugin.command(name="bind")
@click.option("--plugin", required=True, help="Plugin name (e.g. rate-limiting)")
@click.option("--scope", required=True, type=click.Choice(['global', 'provider', 'model', 'tenant', 'role', 'user', 'api_key']), help="Scope level")
@click.option("--scope-id", default="", help="ID of the resource for the scope (leave empty for global)")
@click.option("--config", required=True, help="JSON configuration string for the plugin")
@click.option("--priority", type=int, default=100, help="Execution priority (lower runs first)")
@click.option("--yes", is_flag=True, help="Skip confirmation prompt")
@click.pass_obj
def plugin_bind(client: GatewayClient, plugin: str, scope: str, scope_id: str, config: str, priority: int, yes: bool):
    """Create a new plugin binding to a scope"""
    confirm_destructive(client, f"This will bind plugin {plugin} to {scope} scope.", yes)
    try:
        config_dict = json.loads(config)
    except json.JSONDecodeError as e:
        if client.json_out:
            client.emit({"status": "error", "code": "INVALID_JSON", "message": f"Config must be valid JSON: {e}"})
            return
        rprint(f"[bold red]Invalid JSON config:[/bold red] {e}")
        return

    payload = {
        "plugin_name": plugin,
        "scope_level": scope,
        "scope_id": scope_id,
        "config": config_dict,
        "priority": priority,
        "is_enabled": True
    }

    resp = client.request("POST", "/admin/plugin-bindings", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Plugin Binding Created:[/bold green]")
    rprint(resp.json())

@plugin.command(name="unbind")
@click.option("--id", required=True, help="Binding ID")
@click.option("--yes", is_flag=True, help="Skip confirmation prompt")
@click.pass_obj
def plugin_unbind(client: GatewayClient, id: str, yes: bool):
    """Delete a plugin binding"""
    confirm_destructive(client, f"This will remove plugin binding {id}.", yes)
    resp = client.request("DELETE", f"/admin/plugin-bindings/{id}")
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Plugin Binding Deleted.[/bold green]")

@plugin.command(name="get")
@click.option("--name", required=True, help="Plugin Name")
@click.pass_obj
def plugin_get(client: GatewayClient, name: str):
    """Get detailed information and schema about a plugin"""
    resp = client.request("GET", f"/admin/plugins/{name}").json()
    if client.json_out:
        import json
        print(json.dumps(resp, indent=2))
        return
    rprint(f"[bold green]Plugin:[/bold green] {resp.get('name')}")
    rprint(f"[bold]Description:[/bold] {resp.get('description')}")
    rprint("[bold]Default Config:[/bold]")
    import json
    rprint(json.dumps(resp.get('default_config', {}), indent=2))
