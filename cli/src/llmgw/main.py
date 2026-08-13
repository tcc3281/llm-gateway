import os
import sys
import json
import uuid
from pathlib import Path
import click
import httpx
from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from rich import print as rprint

console = Console()

CONFIG_DIR = Path.home() / ".config" / "llmgw"
CONFIG_FILE = CONFIG_DIR / "config.json"


def load_config() -> tuple[str, str]:
    # 1. Env variables have highest precedence
    base_url = os.getenv("GATEWAY_URL")
    admin_key = os.getenv("ADMIN_API_KEY")

    if base_url and admin_key:
        return base_url, admin_key

    # 2. Try loading from config file
    if CONFIG_FILE.exists():
        try:
            with open(CONFIG_FILE, "r") as f:
                data = json.load(f)
                return (
                    base_url or data.get("gateway_url", "http://localhost:8081"),
                    admin_key or data.get("admin_api_key", "sk-admin-secret-key-change-me")
                )
        except Exception:
            pass

    # 3. Fallbacks
    return base_url or "http://localhost:8081", admin_key or "sk-admin-secret-key-change-me"


def load_public_config() -> tuple[str, str]:
    """Public-proxy connection (for `llmgw chat`). Env > config file > defaults."""
    url = os.getenv("GATEWAY_PUBLIC_URL")
    key = os.getenv("PUBLIC_API_KEY")
    if CONFIG_FILE.exists():
        try:
            with open(CONFIG_FILE, "r") as f:
                data = json.load(f)
                url = url or data.get("public_url")
                key = key or data.get("public_api_key")
        except Exception:
            pass
    return url or "http://localhost:8080", key or ""


# ----------------------------------------------------
# Error helpers
# ----------------------------------------------------

def _extract_error(resp) -> str:
    """Pull a human-readable message from an error response body."""
    try:
        body = resp.json()
    except Exception:
        return resp.text.strip() or f"HTTP {resp.status_code}"
    if isinstance(body, dict):
        for k in ("error", "message", "detail"):
            v = body.get(k)
            if isinstance(v, str):
                return v
            if isinstance(v, dict):
                return v.get("message") or json.dumps(v)
    return resp.text.strip() or f"HTTP {resp.status_code}"


def _status_hint(code: int, msg: str) -> str:
    if code == 401:
        return "Check the admin API key (X-Admin-API-Key); for 'llmgw chat' check the user API key. Run 'llmgw config show'."
    if code == 403:
        return "Client IP not allowed for the admin API (ADMIN_ALLOWED_IPS)."
    if code == 404 and "model" in msg.lower():
        return "Model isn't in the routing catalog — run 'llmgw model discover', then 'llmgw model catalog'."
    if code == 429:
        return "Rate limit hit (key RPM) or budget exhausted."
    return ""


# ----------------------------------------------------
# ID resolution (accept a UUID, a unique prefix, or a human name)
# ----------------------------------------------------

def _is_uuid(s: str) -> bool:
    try:
        uuid.UUID(s)
        return True
    except (ValueError, AttributeError, TypeError):
        return False


def _resolve_id(client: "GatewayClient", arg: str, fetch, name_key: str, what: str, hint: str = "") -> str:
    """Resolve an id argument that may be a UUID, a unique prefix, or a name.

    fetch() returns the parsed list payload; name_key is the field matched for
    names (key -> "name", model -> "model_name", user -> "email").
    """
    if _is_uuid(arg):
        return arg

    rows = fetch() or []
    exact = [r for r in rows if str(r.get(name_key, "")) == arg or str(r.get("id", "")) == arg]
    matches = exact or [
        r for r in rows
        if str(r.get(name_key, "")).startswith(arg) or str(r.get("id", "")).startswith(arg)
    ]

    if len(matches) == 1:
        return str(matches[0]["id"])
    if len(matches) > 1:
        console.print(f"[bold red]'{arg}' matches multiple {what}:[/bold red]")
        for m in matches:
            console.print(f"  {m.get('id')}  {m.get(name_key)}")
        if hint:
            console.print(f"[dim]{hint}[/dim]")
        sys.exit(2)
    console.print(f"[bold red]No {what} found matching '{arg}'.[/bold red]")
    if hint:
        console.print(f"[dim]{hint}[/dim]")
    sys.exit(2)


class GatewayClient:
    def __init__(self, base_url: str, admin_key: str, json_out: bool = False):
        self.base_url = base_url.rstrip("/")
        self.admin_key = admin_key
        self.json_out = json_out
        self.client = httpx.Client(
            headers={
                "Content-Type": "application/json",
                "X-Admin-API-Key": self.admin_key
            },
            timeout=10.0
        )

    def emit(self, data):
        """Print raw JSON when --json is enabled; otherwise the rich render path handles display."""
        if self.json_out:
            print(json.dumps(data, indent=2, default=str))

    def request(self, method: str, path: str, json_data: dict = None, params: dict = None):
        url = f"{self.base_url}{path}"
        try:
            resp = self.client.request(method, url, json=json_data, params=params)
            if resp.status_code >= 400:
                self._die(resp)
            return resp
        except httpx.RequestError as exc:
            if self.json_out:
                print(json.dumps({"ok": False, "error": f"connection_error: {exc}"}, indent=2))
            else:
                console.print(f"[bold red]Connection Error:[/bold red] Failed connecting to Gateway at {self.base_url}: {exc}")
            sys.exit(1)

    def _die(self, resp):
        msg = _extract_error(resp)
        if self.json_out:
            print(json.dumps({"ok": False, "http_status": resp.status_code, "error": msg}, indent=2))
            sys.exit(1)
        console.print(f"[bold red]HTTP {resp.status_code}: {msg}[/bold red]")
        hint = _status_hint(resp.status_code, msg)
        if hint:
            console.print(f"[dim]{hint}[/dim]")
        sys.exit(1)


@click.group()
@click.option("--json", "json_out", is_flag=True, default=False,
              help="Emit raw JSON instead of rich tables (for scripting/automation).")
@click.pass_context
def cli(ctx, json_out):
    """LLM Gateway Command Line Interface (llmgw)"""
    base_url, admin_key = load_config()
    ctx.obj = GatewayClient(base_url, admin_key, json_out=json_out)


# ----------------------------------------------------
# Local Config Commands
# ----------------------------------------------------
@cli.group()
def config():
    """Manage local CLI configuration"""
    pass


@config.command(name="set")
@click.option("--url", help="LLM Gateway Admin API URL")
@click.option("--key", help="Admin API Key")
@click.option("--interactive", is_flag=True, default=False, help="Configure interactively")
@click.pass_context
def config_set(ctx, url: str, key: str, interactive: bool):
    """Save configuration to local config file"""
    if interactive or (not url and not key):
        url = click.prompt("LLM Gateway URL", default=url or "http://localhost:8081")
        key = click.prompt("Admin API Key", default=key or "sk-admin-secret-key-change-me", hide_input=True)

    url = url or "http://localhost:8081"
    key = key or "sk-admin-secret-key-change-me"

    rprint(f"Connecting to Gateway at {url} to verify authentication...")
    client = GatewayClient(url, key)
    try:
        resp = client.client.get(f"{url}/health")
        if resp.status_code == 200:
            rprint("[bold green]✅ Connection and authentication verified successfully![/bold green]")
        else:
            rprint(f"[bold yellow]⚠️ Authentication failed (HTTP {resp.status_code}): {resp.text.strip()}[/bold yellow]")
            if not click.confirm("Do you still want to save this configuration?"):
                sys.exit(1)
    except Exception as exc:
        rprint(f"[bold yellow]⚠️ Failed to connect to host: {exc}[/bold yellow]")
        if not click.confirm("Do you still want to save this configuration?"):
            sys.exit(1)

    try:
        CONFIG_DIR.mkdir(parents=True, exist_ok=True)
        if not CONFIG_FILE.exists():
            CONFIG_FILE.touch(mode=0o600)
        else:
            os.chmod(CONFIG_FILE, 0o600)

        # Preserve any public-proxy settings already saved.
        saved = {}
        try:
            with open(CONFIG_FILE, "r") as f:
                saved = json.load(f)
        except Exception:
            pass
        saved.update({"gateway_url": url, "admin_api_key": key})

        with open(CONFIG_FILE, "w") as f:
            json.dump(saved, f, indent=4)

        rprint(f"[bold green]✅ Configuration saved successfully to {CONFIG_FILE}![/bold green]")
    except Exception as e:
        console.print(f"[bold red]Error saving config file:[/bold red] {e}")
        sys.exit(1)


@config.command(name="show")
@click.pass_obj
def config_show(client: GatewayClient):
    """Display the current active configuration"""
    url, key = load_config()
    pub_url, _ = load_public_config()

    redacted_key = "NOT SET"
    if key:
        if len(key) > 8:
            redacted_key = key[:4] + "..." + key[-4:]
        else:
            redacted_key = "***"

    data = {
        "gateway_url": url,
        "admin_api_key": redacted_key,
        "public_url": pub_url,
        "source": "Environment Variables" if (os.getenv("GATEWAY_URL") or os.getenv("ADMIN_API_KEY")) else
                  (f"Config file ({CONFIG_FILE})" if CONFIG_FILE.exists() else "Default Values"),
    }
    if client.json_out:
        client.emit(data)
        return

    rprint(f"[bold]Active Configuration Source:[/bold]")
    rprint(f"  [cyan]Source:[/cyan] {data['source']}")
    rprint(f"  [cyan]Gateway URL:[/cyan] {url}")
    rprint(f"  [cyan]Admin Key:[/cyan] {redacted_key}")
    rprint(f"  [cyan]Public URL (chat):[/cyan] {pub_url}")


# ----------------------------------------------------
# Health Command
# ----------------------------------------------------
@cli.command()
@click.pass_obj
def health(client: GatewayClient):
    """Check LLM Gateway server connectivity and status"""
    resp = client.request("GET", "/health")
    if client.json_out:
        client.emit(resp.json())
        return
    rprint(f"[bold green]✅ Gateway is healthy![/bold green] Server: {client.base_url}")
    rprint(f"Response: {resp.text.strip()}")


# ----------------------------------------------------
# User Commands
# ----------------------------------------------------
@cli.group()
def user():
    """Manage users"""
    pass


@user.command(name="list")
@click.pass_obj
def user_list(client: GatewayClient):
    """List all users"""
    resp = client.request("GET", "/admin/users")
    users = resp.json()
    if client.json_out:
        client.emit(users)
        return

    if not users:
        rprint("[yellow]⚠️ No users registered yet. Use 'llmgw user create' to add one.[/yellow]")
        return

    table = Table(title="Users List")
    table.add_column("ID", style="cyan", no_wrap=True)
    table.add_column("EMAIL", style="green")
    table.add_column("NAME", style="white")
    table.add_column("ROLE", style="yellow")
    table.add_column("CREATED AT", style="magenta")

    for u in users:
        table.add_row(
            str(u.get("id", "")),
            str(u.get("email", "")),
            str(u.get("name", "")),
            str(u.get("role", "")),
            str(u.get("created_at", ""))
        )
    console.print(table)


@user.command(name="create")
@click.option("--email", required=True, help="User email address")
@click.option("--name", required=True, help="User full name")
@click.option("--role", default="user", show_default=True, help="User role (admin or user)")
@click.pass_obj
def user_create(client: GatewayClient, email: str, name: str, role: str):
    """Create a new user"""
    payload = {"email": email, "name": name, "role": role}
    resp = client.request("POST", "/admin/users", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ User Created Successfully:[/bold green]")
    rprint(resp.json())


# ----------------------------------------------------
# Key Commands
# ----------------------------------------------------
@cli.group()
def key():
    """Manage API keys"""
    pass


@key.command(name="list")
@click.option("--user-id", default="", help="Filter by user UUID or email prefix")
@click.pass_obj
def key_list(client: GatewayClient, user_id: str):
    """List all API keys"""
    params = {}
    if user_id:
        if _is_uuid(user_id):
            params["user_id"] = user_id
        else:
            params["user_id"] = _resolve_id(
                client, user_id,
                fetch=lambda: client.request("GET", "/admin/users").json(),
                name_key="email", what="user", hint="Run 'llmgw user list' to see emails.")
    resp = client.request("GET", "/admin/keys", params=params or None)
    keys = resp.json()
    if client.json_out:
        client.emit(keys)
        return

    if not keys:
        rprint("[yellow]⚠️ No API keys registered yet. Use 'llmgw key create' to generate one.[/yellow]")
        return

    table = Table(title="API Keys List")
    table.add_column("ID", style="cyan", no_wrap=True)
    table.add_column("USER ID", style="magenta")
    table.add_column("NAME", style="white")
    table.add_column("PREFIX", style="green")
    table.add_column("BUDGET (USD)", style="yellow")
    table.add_column("RPM", style="yellow")
    table.add_column("ACTIVE", style="blue")

    for k in keys:
        table.add_row(
            str(k.get("id", "")),
            str(k.get("user_id", "")),
            str(k.get("name", "")),
            str(k.get("key_prefix", "")),
            f"${k.get('budget_limit', 0.0):.2f}",
            str(k.get("rate_limit_rpm", 0)),
            str(k.get("is_active", False))
        )
    console.print(table)


@key.command(name="create")
@click.option("--user-id", required=True, help="User UUID or email prefix")
@click.option("--name", required=True, help="Key name/description")
@click.option("--budget", type=float, default=0.0, show_default=True, help="Budget limit in USD (0 = unlimited)")
@click.option("--rpm", type=int, default=60, show_default=True, help="Rate limit in requests per minute")
@click.pass_obj
def key_create(client: GatewayClient, user_id: str, name: str, budget: float, rpm: int):
    """Create a new API key"""
    if not _is_uuid(user_id):
        user_id = _resolve_id(
            client, user_id,
            fetch=lambda: client.request("GET", "/admin/users").json(),
            name_key="email", what="user", hint="Run 'llmgw user list' to see emails.")
    payload = {
        "user_id": user_id,
        "name": name,
        "budget_limit": budget,
        "rate_limit_rpm": rpm
    }
    resp = client.request("POST", "/admin/keys", json_data=payload)
    result = resp.json()
    if client.json_out:
        client.emit(result)
        return

    rprint("[bold green]🔑 API Key Created Successfully![/bold green]")
    rprint("--------------------------------------------------")
    rprint(f"Secret Key (SAVE THIS NOW!): [bold yellow]{result.get('api_key')}[/bold yellow]")
    rprint("--------------------------------------------------")


@key.command(name="revoke")
@click.option("--id", required=True, help="API key UUID, unique prefix, or key name to revoke")
@click.pass_obj
def key_revoke(client: GatewayClient, id: str):
    """Revoke an API key"""
    key_id = _resolve_id(
        client, id,
        fetch=lambda: client.request("GET", "/admin/keys").json(),
        name_key="name", what="API key", hint="Run 'llmgw key list' to see key names/IDs.")
    resp = client.request("DELETE", "/admin/keys", params={"id": key_id})
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ API Key Revoked:[/bold green]")
    rprint(resp.text.strip())


# ----------------------------------------------------
# Model Commands
# ----------------------------------------------------
@cli.group(invoke_without_command=True)
@click.pass_context
def model(ctx):
    """Manage model pricing and routing. Lists models if no subcommand specified."""
    if ctx.invoked_subcommand is None:
        ctx.invoke(model_list)


@model.command(name="list")
@click.pass_obj
def model_list(client: GatewayClient):
    """List registered model pricing rules"""
    resp = client.request("GET", "/admin/models")
    models = resp.json()
    if client.json_out:
        client.emit(models)
        return

    if not models:
        rprint("[yellow]⚠️ No models registered yet. Use 'llmgw model sync' or 'llmgw model add' to configure pricing rules.[/yellow]")
        return

    table = Table(title="Models & Pricing List")
    table.add_column("ID", style="cyan", no_wrap=True)
    table.add_column("PROVIDER", style="yellow")
    table.add_column("MODEL NAME", style="white")
    table.add_column("PROMPT / 1M", style="green")
    table.add_column("COMPLETION / 1M", style="green")

    for m in models:
        table.add_row(
            str(m.get("id", "")),
            str(m.get("provider", "")),
            str(m.get("model_name", "")),
            f"${m.get('prompt_price_per_1m', 0.0):.4f}",
            f"${m.get('completion_price_per_1m', 0.0):.4f}"
        )
    console.print(table)


@model.command(name="catalog")
@click.option("--provider", default="", help="Filter by provider name")
@click.pass_obj
def model_catalog(client: GatewayClient, provider: str):
    """Show the full routable catalog: discovered (provider_models) + registered pricing"""
    resp = client.request("GET", "/admin/models/catalog")
    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    registered = data.get("registered", []) or []
    discovered = data.get("discovered", []) or []

    if not registered and not discovered:
        rprint("[yellow]⚠️ Catalog is empty. Run 'llmgw model discover' (routing) and/or 'llmgw model add' (pricing).[/yellow]")
        return

    prices = {(r.get("provider"), r.get("model_name")): r for r in registered}
    rows = {}
    for d in discovered:
        rows[(d.get("provider"), d.get("model_name"))] = {
            "model": d.get("model_name", ""),
            "provider": d.get("provider", ""),
            "source": ["discovered"],
        }
    for r in registered:
        key = (r.get("provider"), r.get("model_name"))
        if key in rows:
            rows[key]["source"].append("registered")
        else:
            rows[key] = {
                "model": r.get("model_name", ""),
                "provider": r.get("provider", ""),
                "source": ["registered"],
            }

    ordered = sorted(rows.values(), key=lambda x: (x["provider"], x["model"]))
    if provider:
        ordered = [r for r in ordered if r["provider"] == provider]
    if not ordered:
        rprint(f"[yellow]⚠️ No catalog entries for provider '{provider}'.[/yellow]")
        return

    table = Table(title=f"Routable Model Catalog{f' — {provider}' if provider else ''}")
    table.add_column("MODEL", style="white", no_wrap=True)
    table.add_column("PROVIDER", style="yellow")
    table.add_column("SOURCE", style="cyan")
    table.add_column("PROMPT / 1M", style="green")
    table.add_column("COMPLETION / 1M", style="green")

    for r in ordered:
        price = prices.get((r["provider"], r["model"]))
        src = "+".join(sorted(r["source"]))
        if price is not None:
            pp = f"${price.get('prompt_price_per_1m', 0.0):.4f}"
            cp = f"${price.get('completion_price_per_1m', 0.0):.4f}"
        else:
            pp = "—"
            cp = "—"
        table.add_row(r["model"], r["provider"], src, pp, cp)
    console.print(table)
    console.print("[dim]discovered = routable via the provider's /models API · registered = has explicit pricing for cost · --provider X to filter[/dim]")


@model.command(name="add")
@click.option("--provider", required=True, help="Model provider (built-in or custom compatible provider name)")
@click.option("--name", required=True, help="Model name (e.g. gpt-4o)")
@click.option("--prompt-price", type=float, default=0.0, show_default=True, help="Price per 1M prompt tokens in USD")
@click.option("--completion-price", type=float, default=0.0, show_default=True, help="Price per 1M completion tokens in USD")
@click.pass_obj
def model_add(client: GatewayClient, provider: str, name: str, prompt_price: float, completion_price: float):
    """Add or update model pricing"""
    payload = {
        "provider": provider,
        "model_name": name,
        "prompt_price_per_1m": prompt_price,
        "completion_price_per_1m": completion_price
    }
    resp = client.request("POST", "/admin/models", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Model Pricing Saved:[/bold green]")
    rprint(resp.json())


@model.command(name="delete")
@click.option("--id", required=True, help="Model UUID, unique prefix, or model name to delete")
@click.pass_obj
def model_delete(client: GatewayClient, id: str):
    """Delete a model pricing rule"""
    model_id = _resolve_id(
        client, id,
        fetch=lambda: client.request("GET", "/admin/models").json(),
        name_key="model_name", what="model", hint="Run 'llmgw model list' to see model names/IDs.")
    resp = client.request("DELETE", "/admin/models", params={"id": model_id})
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Model Deleted:[/bold green]")
    rprint(resp.text.strip())


@model.command(name="sync")
@click.option("--provider", default="all", type=click.Choice(["openai", "anthropic", "gemini", "all"]), show_default=True, help="Provider to sync prices from the LiteLLM catalog")
@click.pass_obj
def model_sync(client: GatewayClient, provider: str):
    """Pull pricing rules from the LiteLLM catalog into model_pricings (cost calculation only — does NOT affect routing)"""
    with console.status(f"[bold green]Triggering model pricing sync for '{provider}' on Gateway...") as status:
        resp = client.request("POST", "/admin/models/sync", params={"provider": provider})
        result = resp.json()

    if client.json_out:
        client.emit(result)
        return
    synced_models = result.get("synced_models", 0)
    rprint(f"[bold green]🎉 Done! Gateway successfully synchronized {synced_models} models and pricing rules with the database.[/bold green]")


@model.command(name="discover")
@click.pass_obj
def model_discover(client: GatewayClient):
    """Fetch each configured provider's /models API into provider_models (the ROUTING catalog)"""
    with console.status("[bold green]Discovering models from configured providers...") as status:
        resp = client.request("POST", "/admin/models/discover")
        result = resp.json()

    if client.json_out:
        client.emit(result)
        return
    synced = result.get("synced_models", 0)
    errors = result.get("errors", {})
    if errors:
        rprint(f"[bold yellow]⚠️ Discovered {synced} models with some provider errors:[/bold yellow]")
        for prov, msg in errors.items():
            rprint(f"  [red]{prov}: {msg}[/red]")
    else:
        rprint(f"[bold green]🎉 Discovered {synced} models across providers into the routing catalog.[/bold green]")
    rprint("[dim]See the list with 'llmgw model catalog'.[/dim]")


# ----------------------------------------------------
# Provider Commands
# ----------------------------------------------------
@cli.group(invoke_without_command=True)
@click.pass_context
def provider(ctx):
    """Manage upstream provider API keys. Lists providers if no subcommand specified."""
    if ctx.invoked_subcommand is None:
        ctx.invoke(provider_list)


@provider.command(name="list")
@click.pass_obj
def provider_list(client: GatewayClient):
    """List upstream provider configurations"""
    resp = client.request("GET", "/admin/providers")
    providers = resp.json()
    if client.json_out:
        client.emit(providers)
        return

    if not providers:
        rprint("[yellow]⚠️ No upstream provider credentials configured. Use 'llmgw provider set' to add keys.[/yellow]")
        return

    table = Table(title="Provider API Keys")
    table.add_column("PROVIDER", style="yellow", no_wrap=True)
    table.add_column("BASE URL", style="cyan")
    table.add_column("PROTOCOL", style="blue")
    table.add_column("KEY PREVIEW", style="green")
    table.add_column("ACTIVE", style="blue")
    table.add_column("UPDATED AT", style="magenta")

    for p in providers:
        table.add_row(
            str(p.get("provider", "")),
            str(p.get("base_url", "") or "-"),
            str(p.get("protocol", "") or "-"),
            str(p.get("key_preview", "")),
            str(p.get("is_active", False)),
            str(p.get("updated_at", ""))
        )
    console.print(table)


@provider.command(name="set")
@click.option("--provider", required=True, help="Provider name (built-in or custom compatible provider name)")
@click.option("--api-key", required=True, help="Upstream API key")
@click.option("--base-url", default="", help="Custom upstream base URL for OpenAI/Anthropic-compatible providers (empty = built-in fixed URL)")
@click.option("--protocol", default="openai", type=click.Choice(["openai", "anthropic"]), show_default=True, help="Wire protocol spoken to a custom base_url")
@click.pass_obj
def provider_set(client: GatewayClient, provider: str, api_key: str, base_url: str, protocol: str):
    """Set API key for an upstream provider"""
    payload = {"provider": provider, "api_key": api_key, "base_url": base_url, "protocol": protocol}
    resp = client.request("POST", "/admin/providers", json_data=payload)
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Provider API key saved:[/bold green]")
    rprint(resp.json())


@provider.command(name="delete")
@click.option("--provider", required=True, help="Provider to delete")
@click.pass_obj
def provider_delete(client: GatewayClient, provider: str):
    """Delete API key for an upstream provider"""
    resp = client.request("DELETE", "/admin/providers", params={"provider": provider})
    if client.json_out:
        client.emit(resp.json())
        return
    rprint("[bold green]✅ Provider Key Deleted:[/bold green]")
    rprint(resp.text.strip())


# ----------------------------------------------------
# Stats Command
# ----------------------------------------------------
@cli.command()
@click.option("--interval", default="hour", type=click.Choice(["hour", "day"]), show_default=True, help="Time interval")
@click.option("--user-id", default="", help="Filter stats by user UUID or email prefix")
@click.pass_obj
def stats(client: GatewayClient, interval: str, user_id: str):
    """View timeseries usage analytics and telemetry"""
    params = {"interval": interval}
    if user_id:
        if _is_uuid(user_id):
            params["user_id"] = user_id
        else:
            params["user_id"] = _resolve_id(
                client, user_id,
                fetch=lambda: client.request("GET", "/admin/users").json(),
                name_key="email", what="user", hint="Run 'llmgw user list' to see emails.")

    resp = client.request("GET", "/admin/analytics/stats", params=params)
    stats_resp = resp.json()
    if client.json_out:
        client.emit(stats_resp)
        return

    rprint(f"[bold]📊 LLM Gateway Usage & Analytics Report[/bold]")
    rprint(f"Interval: [cyan]{stats_resp.get('interval')}[/cyan] | From: {stats_resp.get('from')} | To: {stats_resp.get('to')}\n")

    data = stats_resp.get("data", [])
    if not data:
        rprint("[yellow]No telemetry records found for this period.[/yellow]")
        return

    table = Table(title="Telemetry Records")
    table.add_column("TIME BUCKET", style="cyan")
    table.add_column("PROVIDER", style="yellow")
    table.add_column("MODEL", style="white")
    table.add_column("REQUESTS", style="green")
    table.add_column("PROMPT TOKENS", style="green")
    table.add_column("COMPLETION TOKENS", style="green")
    table.add_column("TOTAL COST", style="yellow")
    table.add_column("AVG TTFT", style="blue")
    table.add_column("AVG LATENCY", style="blue")

    for item in data:
        table.add_row(
            str(item.get("time_bucket", "")),
            str(item.get("provider", "")),
            str(item.get("model", "")),
            str(item.get("total_requests", 0)),
            str(item.get("sum_prompt_tokens", 0)),
            str(item.get("sum_completion_tokens", 0)),
            f"${item.get('total_cost', 0.0):.4f}",
            f"{item.get('avg_ttft_ms', 0.0):.1f} ms",
            f"{item.get('avg_latency_ms', 0.0):.1f} ms"
        )
    console.print(table)


# ----------------------------------------------------
# Logs Command
# ----------------------------------------------------
@cli.command()
@click.option("--limit", type=int, default=50, show_default=True, help="Max rows to show (≤200)")
@click.option("--status", default="", help="Filter by status code or class: '200', '2xx', '4xx', '5xx'")
@click.option("--model", default="", help="Filter by model name")
@click.option("--provider", default="", help="Filter by provider")
@click.option("--user-id", default="", help="Filter by user UUID or email prefix")
@click.pass_obj
def logs(client: GatewayClient, limit: int, status: str, model: str, provider: str, user_id: str):
    """View gateway usage logs (GET /admin/logs)"""
    params = {"limit": limit}
    if status:
        params["status"] = status
    if model:
        params["model"] = model
    if provider:
        params["provider"] = provider
    if user_id:
        if _is_uuid(user_id):
            params["user_id"] = user_id
        else:
            params["user_id"] = _resolve_id(
                client, user_id,
                fetch=lambda: client.request("GET", "/admin/users").json(),
                name_key="email", what="user", hint="Run 'llmgw user list' to see emails.")

    resp = client.request("GET", "/admin/logs", params=params)
    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    rows = data.get("data", []) or []
    total = data.get("total", 0)
    if not rows:
        rprint("[yellow]No usage logs match this filter.[/yellow]")
        return

    table = Table(title=f"Usage Logs ({total} total)")
    table.add_column("TIME", style="magenta")
    table.add_column("USER", style="cyan")
    table.add_column("PROVIDER", style="yellow")
    table.add_column("MODEL", style="white")
    table.add_column("STATUS", style="blue")
    table.add_column("COST", style="yellow")
    table.add_column("TTFT", style="blue")
    table.add_column("LATENCY", style="blue")
    table.add_column("ERROR", style="red")

    for log in rows:
        code = log.get("status_code", 200)
        code_style = "green" if 200 <= code < 300 else "red"
        ttft = log.get("ttft_ms")
        ttft_s = f"{ttft:.1f} ms" if ttft is not None else "N/A"
        lat = log.get("total_latency_ms")
        lat_s = f"{lat:.1f} ms" if lat is not None else "N/A"
        table.add_row(
            str(log.get("created_at", "")),
            str(log.get("user_id", "")),
            str(log.get("provider", "")),
            str(log.get("model", "")),
            f"[{code_style}]{code}[/{code_style}]",
            f"${log.get('cost', 0.0):.4f}",
            ttft_s,
            lat_s,
            str(log.get("error_message", "") or "")
        )
    console.print(table)


# ----------------------------------------------------
# Consolidated Dashboard Command
# ----------------------------------------------------
@cli.command()
@click.option("--interval", default="hour", type=click.Choice(["hour", "day"]), show_default=True, help="Time bucket interval")
@click.option("--user-id", default="", help="Filter stats by user UUID or email prefix")
@click.option("--limit", type=int, default=10, show_default=True, help="Number of recent logs to show")
@click.pass_obj
def dashboard(client: GatewayClient, interval: str, user_id: str, limit: int):
    """View consolidated real-time dashboard analytics"""
    params = {"interval": interval, "limit": limit}
    if user_id:
        if _is_uuid(user_id):
            params["user_id"] = user_id
        else:
            params["user_id"] = _resolve_id(
                client, user_id,
                fetch=lambda: client.request("GET", "/admin/users").json(),
                name_key="email", what="user", hint="Run 'llmgw user list' to see emails.")

    resp = client.request("GET", "/admin/analytics/dashboard", params=params)
    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    # 1. Totals Panel
    totals = data.get("totals", {}) or {}
    req_count = totals.get("requests", 0)
    cost = totals.get("cost_usd", 0.0)
    prompt_tokens = totals.get("prompt_tokens", 0)
    completion_tokens = totals.get("completion_tokens", 0)
    total_tokens = prompt_tokens + completion_tokens
    avg_ttft = totals.get("avg_ttft_ms", 0.0)
    avg_latency = totals.get("avg_latency_ms", 0.0)
    errors = totals.get("errors", 0)
    error_rate = (totals.get("error_rate", 0.0) or 0.0) * 100.0

    totals_content = (
        f"[bold]Requests:[/bold] {req_count} | [bold]Errors:[/bold] {errors} ({error_rate:.1f}%)\n"
        f"[bold]Total Cost:[/bold] [yellow]${cost:.4f}[/yellow]\n"
        f"[bold]Tokens:[/bold] {total_tokens} (Prompt: {prompt_tokens} / Comp: {completion_tokens})\n"
        f"[bold]Avg TTFT:[/bold] {avg_ttft:.1f} ms | [bold]Avg Latency:[/bold] {avg_latency:.1f} ms"
    )

    rprint(Panel(totals_content, title="📊 LLM Gateway Dashboard - Key Metrics Summary", border_style="green"))

    # 2. Top Models
    by_model = data.get("by_model", []) or []
    if by_model:
        model_table = Table(title="🔥 Usage by Model")
        model_table.add_column("Provider", style="yellow")
        model_table.add_column("Model Name", style="white")
        model_table.add_column("Requests", style="green")
        model_table.add_column("Cost (USD)", style="yellow")
        model_table.add_column("Avg Latency", style="blue")
        for m in by_model:
            model_table.add_row(
                str(m.get("provider", "")),
                str(m.get("model", "")),
                str(m.get("requests", 0)),
                f"${m.get('cost_usd', 0.0):.4f}",
                f"{m.get('avg_latency_ms', 0.0):.1f} ms"
            )
        console.print(model_table)

    # 3. Top Users
    by_user = data.get("by_user", []) or []
    if by_user:
        user_table = Table(title="👥 Usage by User")
        user_table.add_column("User ID", style="cyan")
        user_table.add_column("Email", style="green")
        user_table.add_column("Requests", style="green")
        user_table.add_column("Cost (USD)", style="yellow")
        for u in by_user:
            user_table.add_row(
                str(u.get("user_id", "")),
                str(u.get("email", "")),
                str(u.get("requests", 0)),
                f"${u.get('cost_usd', 0.0):.4f}"
            )
        console.print(user_table)

    # 4. Recent Logs
    recent = data.get("recent", []) or []
    if recent:
        recent_table = Table(title="⏱️ Recent Logs")
        recent_table.add_column("Created At", style="magenta")
        recent_table.add_column("User ID", style="cyan")
        recent_table.add_column("Provider", style="yellow")
        recent_table.add_column("Model", style="white")
        recent_table.add_column("Status", style="blue")
        recent_table.add_column("Cost", style="yellow")
        recent_table.add_column("TTFT", style="blue")
        recent_table.add_column("Latency", style="blue")
        for log in recent:
            status = log.get("status_code", 200)
            status_style = "green" if 200 <= status < 300 else "red"

            ttft_val = log.get('ttft_ms')
            ttft_str = f"{ttft_val:.1f} ms" if ttft_val is not None else "N/A"

            lat_val = log.get('total_latency_ms')
            lat_str = f"{lat_val:.1f} ms" if lat_val is not None else "N/A"

            recent_table.add_row(
                str(log.get("created_at", "")),
                str(log.get("user_id", "")),
                str(log.get("provider", "")),
                str(log.get("model", "")),
                f"[{status_style}]{status}[/{status_style}]",
                f"${log.get('cost_usd', 0.0):.4f}",
                ttft_str,
                lat_str
            )
        console.print(recent_table)


# ----------------------------------------------------
# Chat Command — smoke test the PUBLIC proxy (port 8080)
# ----------------------------------------------------

def _openai_message_content(data: dict) -> tuple[str, str]:
    """(content, reasoning) from a non-stream OpenAI-format response."""
    choices = data.get("choices") or []
    msg = choices[0].get("message", {}) if choices else {}
    return (msg.get("content") or "", msg.get("reasoning_content") or "")


def _anthropic_message_content(data: dict) -> tuple[str, str]:
    """(content, reasoning) from a non-stream Anthropic-format response."""
    text, reasoning = [], []
    for block in data.get("content") or []:
        btype = block.get("type")
        if btype in ("thinking", "redacted_thinking"):
            reasoning.append(block.get("thinking") or "")
        elif btype == "text":
            text.append(block.get("text") or "")
    return "".join(text), "".join(reasoning)


def _delta_content(chunk: dict, inbound: str) -> tuple[str, str]:
    """(text_delta, reasoning_delta) from one SSE chunk in the given inbound format."""
    if inbound == "anthropic":
        if chunk.get("type") != "content_block_delta":
            return "", ""
        delta = chunk.get("delta") or {}
        if delta.get("type") == "thinking_delta":
            return "", delta.get("thinking") or ""
        if delta.get("type") == "text_delta":
            return delta.get("text") or "", ""
        return "", ""
    # openai
    choices = chunk.get("choices") or []
    d = choices[0].get("delta", {}) if choices else {}
    return d.get("content") or "", d.get("reasoning_content") or ""


def _chat_usage(data: dict, inbound: str) -> dict:
    usage = data.get("usage") or {}
    if inbound == "anthropic":
        return {
            "input": usage.get("input_tokens"),
            "output": usage.get("output_tokens"),
        }
    return {
        "input": usage.get("prompt_tokens"),
        "output": usage.get("completion_tokens"),
    }


def _print_tool_calls(tool_calls: list) -> None:
    """Print assistant tool_calls from a non-stream OpenAI response."""
    for tc in tool_calls or []:
        fn = tc.get("function") or {}
        console.print(f"[cyan]⚙️  tool_call {tc.get('id')}[/cyan] {fn.get('name')}({fn.get('arguments')})")


def _print_tool_calls_from_dict(stream_tool_calls: dict) -> None:
    """Print tool calls reconstructed from streaming fragments (by chunk index)."""
    for idx in sorted(stream_tool_calls):
        e = stream_tool_calls[idx]
        if not e.get("name"):
            continue
        console.print(f"[cyan]⚙️  tool_call {e.get('id')}[/cyan] {e.get('name')}({e.get('arguments')})")


@cli.command()
@click.option("--url", default="", help="Public gateway URL (default: GATEWAY_PUBLIC_URL env, then http://localhost:8080)")
@click.option("--api-key", default="", help="User API key (sk-gw-...). Default: PUBLIC_API_KEY env.")
@click.option("--model", required=True, help="Model name to call (must be in the routing catalog)")
@click.option("--message", default=None, help="User message to send (default: a tool prompt with --tools, else a hello)")
@click.option("--max-tokens", type=int, default=200, show_default=True, help="Max output tokens")
@click.option("--stream", is_flag=True, default=False, help="Stream tokens as SSE arrive (realtime)")
@click.option("--inbound", "inbound_format", type=click.Choice(["openai", "anthropic"]), default="openai",
              show_default=True, help="Wire protocol to speak to the public proxy")
@click.option("--show-reasoning", is_flag=True, default=False,
              help="Also print reasoning/thinking content (OpenAI reasoning_content / Anthropic thinking blocks)")
@click.option("--reasoning-effort", type=click.Choice(["low", "medium", "high"]), default=None,
              help="Enable upstream thinking: OpenAI reasoning_effort, Anthropic thinking:enabled (passthrough)")
@click.option("--tools", is_flag=True, default=False,
              help="Inject a get_weather test tool. OpenAI inbound auto round-trips the tool result; Anthropic inbound prints tool_use blocks.")
@click.pass_obj
def chat(client: GatewayClient, url: str, api_key: str, model: str, message: str,
         max_tokens: int, stream: bool, inbound_format: str, show_reasoning: bool, reasoning_effort: str, tools: bool):
    """Smoke-test the PUBLIC proxy with a real model call — verify routing end-to-end after config changes."""
    pub_url, default_key = load_public_config()
    pub_url = url or pub_url
    key = api_key or default_key
    if not key:
        console.print("[bold red]No API key for the public proxy.[/bold red]")
        console.print("[dim]Pass --api-key (sk-gw-...) or set PUBLIC_API_KEY. Create one with 'llmgw key create'.[/dim]")
        sys.exit(1)

    if message is None:
        message = ("What is the weather in Hanoi right now? Call the get_weather tool."
                   if tools else "Say hello in one short sentence.")

    headers = {"Content-Type": "application/json"}
    if inbound_format == "anthropic":
        endpoint = f"{pub_url.rstrip('/')}/v1/messages"
        headers["x-api-key"] = key
        headers["anthropic-version"] = "2023-06-01"
        body = {"model": model, "max_tokens": max_tokens,
                "messages": [{"role": "user", "content": message}], "stream": stream}
        if reasoning_effort:
            body["thinking"] = {"type": "enabled"}
        if tools:
            body["tools"] = [{
                "name": "get_weather",
                "description": "Get the current weather for a location",
                "input_schema": {"type": "object", "properties": {"location": {"type": "string"}},
                                 "required": ["location"]},
            }]
    else:
        endpoint = f"{pub_url.rstrip('/')}/v1/chat/completions"
        headers["Authorization"] = f"Bearer {key}"
        body = {"model": model, "messages": [{"role": "user", "content": message}],
                "max_tokens": max_tokens, "stream": stream}
        if reasoning_effort:
            body["reasoning_effort"] = reasoning_effort
        if tools:
            body["tools"] = [{
                "type": "function",
                "function": {
                    "name": "get_weather",
                    "description": "Get the current weather for a location",
                    "parameters": {"type": "object", "properties": {"location": {"type": "string"}},
                                   "required": ["location"]},
                },
            }]

    timeout = httpx.Timeout(180.0, read=120.0)

    # ---- Streaming path: print deltas as they arrive; --json emits each SSE payload line.
    # Tool-call fragments accumulate across chunks (OpenAI delta.tool_calls,
    # Anthropic content_block_start/input_json_delta) and print after the stream.
    stream_tool_calls: dict = {}
    if stream:
        try:
            with httpx.stream("POST", endpoint, json=body, headers=headers, timeout=timeout) as resp:
                if resp.status_code >= 400:
                    body_text = resp.read().decode("utf-8", "replace")
                    msg = body_text.strip()
                    if client.json_out:
                        print(json.dumps({"ok": False, "http_status": resp.status_code, "error": msg}, indent=2))
                    else:
                        console.print(f"[bold red]HTTP {resp.status_code}: {msg}[/bold red]")
                        hint = _status_hint(resp.status_code, msg)
                        if hint:
                            console.print(f"[dim]{hint}[/dim]")
                    sys.exit(1)
                for line in resp.iter_lines():
                    line = (line or "").strip()
                    if not line.startswith("data:"):
                        continue
                    payload = line[len("data:"):].strip()
                    if not payload:
                        continue
                    if payload == "[DONE]":
                        break
                    if client.json_out:
                        print(payload)
                        continue
                    try:
                        chunk = json.loads(payload)
                    except json.JSONDecodeError:
                        continue
                    if tools and inbound_format == "openai":
                        for dtc in (chunk.get("choices") or [{}])[0].get("delta", {}).get("tool_calls") or []:
                            entry = stream_tool_calls.setdefault(dtc.get("index", 0),
                                                                {"id": "", "name": "", "arguments": ""})
                            if dtc.get("id"):
                                entry["id"] = dtc["id"]
                            fn = dtc.get("function") or {}
                            if fn.get("name"):
                                entry["name"] = fn["name"]
                            entry["arguments"] += fn.get("arguments") or ""
                    elif tools and inbound_format == "anthropic":
                        if chunk.get("type") == "content_block_start":
                            cb = chunk.get("content_block") or {}
                            if cb.get("type") == "tool_use":
                                idx = chunk.get("index", 0)
                                entry = stream_tool_calls.setdefault(idx, {"id": "", "name": "", "arguments": ""})
                                entry["id"] = cb.get("id") or ""
                                entry["name"] = cb.get("name") or ""
                        elif chunk.get("type") == "content_block_delta":
                            delta = chunk.get("delta") or {}
                            if delta.get("type") == "input_json_delta":
                                idx = chunk.get("index", 0)
                                entry = stream_tool_calls.setdefault(idx, {"id": "", "name": "", "arguments": ""})
                                entry["arguments"] += delta.get("partial_json") or ""
                    text, reason = _delta_content(chunk, inbound_format)
                    if show_reasoning and reason:
                        sys.stdout.write(reason)
                        sys.stdout.flush()
                    if text:
                        sys.stdout.write(text)
                        sys.stdout.flush()
        except httpx.RequestError as exc:
            if client.json_out:
                print(json.dumps({"ok": False, "error": f"connection_error: {exc}"}, indent=2))
            else:
                console.print(f"[bold red]Connection Error:[/bold red] failed to reach public proxy at {pub_url}: {exc}")
            sys.exit(1)
        if not client.json_out:
            sys.stdout.write("\n")
            sys.stdout.flush()
            if tools and stream_tool_calls:
                _print_tool_calls_from_dict(stream_tool_calls)
            console.print(f"[dim]streamed {model} via {inbound_format} ({pub_url})[/dim]")
        return

    # ---- Non-stream path.
    try:
        resp = httpx.post(endpoint, json=body, headers=headers, timeout=timeout)
    except httpx.RequestError as exc:
        if client.json_out:
            print(json.dumps({"ok": False, "error": f"connection_error: {exc}"}, indent=2))
        else:
            console.print(f"[bold red]Connection Error:[/bold red] failed to reach public proxy at {pub_url}: {exc}")
        sys.exit(1)

    if resp.status_code >= 400:
        msg = _extract_error(resp)
        if client.json_out:
            print(json.dumps({"ok": False, "http_status": resp.status_code, "error": msg}, indent=2))
        else:
            console.print(f"[bold red]HTTP {resp.status_code}: {msg}[/bold red]")
            hint = _status_hint(resp.status_code, msg)
            if hint:
                console.print(f"[dim]{hint}[/dim]")
        sys.exit(1)

    data = resp.json()
    if client.json_out:
        client.emit(data)
        return

    if inbound_format == "anthropic":
        content, reasoning = _anthropic_message_content(data)
        tool_uses = [b for b in (data.get("content") or []) if b.get("type") == "tool_use"]
        if tools and tool_uses:
            for b in tool_uses:
                console.print(f"[cyan]⚙️  tool_use {b.get('id')}[/cyan] {b.get('name')}({json.dumps(b.get('input'))})")
    else:
        content, reasoning = _openai_message_content(data)
        tool_calls = (data.get("choices") or [{}])[0].get("message", {}).get("tool_calls") or []
        if tools and tool_calls:
            _print_tool_calls(tool_calls)

    if show_reasoning and reasoning:
        console.print(Panel(reasoning, title="🧠 Reasoning", border_style="dim"))
    if content:
        console.print(content)

    # OpenAI inbound: auto round-trip the tool result so the model can answer.
    if tools and inbound_format == "openai" and tool_calls:
        assistant_msg = (data.get("choices") or [{}])[0].get("message", {})
        messages2 = list(body["messages"]) + [assistant_msg]
        for tc in tool_calls:
            messages2.append({"role": "tool", "tool_call_id": tc.get("id"), "content": "Sunny, 25°C"})
        body2 = dict(body)
        body2["messages"] = messages2
        body2["stream"] = False
        try:
            resp2 = httpx.post(endpoint, json=body2, headers=headers, timeout=timeout)
        except httpx.RequestError as exc:
            console.print(f"[bold red]Round-trip failed (connection): {exc}[/bold red]")
            sys.exit(1)
        if resp2.status_code >= 400:
            console.print(f"[bold red]Round-trip failed: {_extract_error(resp2)}[/bold red]")
            sys.exit(1)
        data2 = resp2.json()
        final_content, _ = _openai_message_content(data2)
        if final_content:
            console.print(final_content)
        console.print("[dim]tool round-trip OK — model used the tool result[/dim]")
        return

    usage = _chat_usage(data, inbound_format)
    parts = [f"model={model}"]
    if usage.get("input") is not None:
        parts.append(f"input_tokens={usage['input']}")
    if usage.get("output") is not None:
        parts.append(f"output_tokens={usage['output']}")
    console.print(f"[dim]{' | '.join(parts)} ({pub_url})[/dim]")


if __name__ == "__main__":
    cli()
