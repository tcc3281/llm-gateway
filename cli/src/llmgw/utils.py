import json
import sys
import uuid

import click
from rich.console import Console

console = Console()


def is_uuid(s: str) -> bool:
    try:
        uuid.UUID(s)
        return True
    except (ValueError, AttributeError, TypeError):
        return False


def resolve_id(client, arg: str, fetch, name_key: str, what: str, hint: str = "") -> str:
    """Resolve an id argument that may be a UUID, a unique prefix, or a name.

    fetch() returns the parsed list payload; name_key is the field matched for
    names (key -> "name", model -> "model_name", user -> "email").
    """
    if is_uuid(arg):
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
        if client.json_out:
            print('{"status": "error", "code": "AMBIGUOUS_MATCH", "message": "Multiple matches found"}')
            sys.exit(2)
        console.print(f"[bold red]'{arg}' matches multiple {what}:[/bold red]")
        for m in matches:
            console.print(f"  {m.get('id')}  {m.get(name_key)}")
        if hint:
            console.print(f"[dim]{hint}[/dim]")
        sys.exit(2)

    if client.json_out:
        print(f'{{"status": "error", "code": "NOT_FOUND", "message": "No {what} found matching \'{arg}\'"}}')
        sys.exit(2)
    console.print(f"[bold red]No {what} found matching '{arg}'.[/bold red]")
    if hint:
        console.print(f"[dim]{hint}[/dim]")
    sys.exit(2)


def confirm_destructive(client, prompt: str, yes: bool):
    """Ensure hard gates for destructive commands."""
    if yes:
        return
    if client.json_out:
        print(json.dumps({
            "status": "error",
            "code": "CONFIRMATION_REQUIRED",
            "message": "Destructive operation requires --yes flag",
        }))
        sys.exit(2)
    click.confirm(f"[Warning] {prompt} Are you sure?", abort=True)
