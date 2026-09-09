import json
import os
import sys
from pathlib import Path

import httpx
from rich.console import Console

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

def _extract_error(resp) -> str:
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
                print(json.dumps({
                    "status": "error",
                    "code": "CONNECTION_ERROR",
                    "message": f"Failed connecting to Gateway at {self.base_url}: {exc}",
                    "suggested_action": "Check if llm-gateway is running and GATEWAY_URL is correct."
                }, indent=2))
            else:
                console.print(f"[bold red]Connection Error:[/bold red] Failed connecting to Gateway at {self.base_url}: {exc}")
            sys.exit(1)

    def _die(self, resp):
        msg = _extract_error(resp)
        if self.json_out:
            print(json.dumps({
                "status": "error",
                "code": f"HTTP_{resp.status_code}",
                "message": msg,
                "suggested_action": "Check parameters or permissions."
            }, indent=2))
            sys.exit(1)
        console.print(f"[bold red]HTTP {resp.status_code}: {msg}[/bold red]")
        sys.exit(1)

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
