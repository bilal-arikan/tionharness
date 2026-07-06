#!/usr/bin/env python3
"""Migrate the TS gateway-manager config.json into TionSwarm MCP servers (Doc 52 Faz 3).

Reads mcp-server/config.json (+ optional secrets.json) and emits a normalized
{"mcpServers": {...}} document in the shape TionSwarm's POST /api/mcp-servers/import
accepts (the standard Claude Code / .mcp.json form). Then the whole 18+7-server gateway
config lands in a workspace, and an external client points at /mcp/gateway instead of the
TS gateway — the TS gateway-manager can be retired.

Transforms applied:
  - transportType "streamable-http"/"sse" or a url  -> type:"http" (import infers http from url)
  - ${VAR} placeholders in env/headers               -> resolved from secrets.json / the environment
  - options.disabled                                 -> tracked (reported), entry still emitted so
                                                        the operator can toggle it off in the UI
  - options (and other gateway-only keys)            -> dropped (TionSwarm has no equivalent)

Usage:
  python migrate-vps.py <config.json> [--secrets secrets.json] [--enabled-only] > import.json
  # then: curl -X POST http://127.0.0.1:8090/api/mcp-servers/import \
  #            -H "X-Workspace-Id: <ws>" --data-binary @import.json
"""
import argparse
import json
import os
import re
import sys

PLACEHOLDER = re.compile(r"\$\{([A-Za-z0-9_]+)\}")


def resolve(value, secrets):
    """Resolve ${VAR} placeholders in a string from secrets then the environment."""
    if not isinstance(value, str):
        return value
    def repl(m):
        key = m.group(1)
        return str(secrets.get(key, os.environ.get(key, m.group(0))))
    return PLACEHOLDER.sub(repl, value)


def resolve_map(d, secrets):
    return {k: resolve(v, secrets) for k, v in (d or {}).items()}


def normalize(name, spec, secrets):
    """Return (normalized_spec, disabled, warnings) for one TS server entry."""
    warnings = []
    disabled = bool((spec.get("options") or {}).get("disabled"))
    out = {}
    url = spec.get("url")
    transport = spec.get("transportType")
    if url or transport:
        out["type"] = "http"  # TionSwarm infers http from url; sse is not supported
        if transport == "sse":
            warnings.append("sse transport downgraded to http (TionSwarm import rejects sse)")
        if url:
            out["url"] = url
        if spec.get("headers"):
            out["headers"] = resolve_map(spec["headers"], secrets)
    else:
        out["type"] = "stdio"
        out["command"] = spec.get("command", "")
        if spec.get("args"):
            out["args"] = spec["args"]
        if spec.get("env"):
            out["env"] = resolve_map(spec["env"], secrets)
    return out, disabled, warnings


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("config", help="path to the TS gateway config.json")
    ap.add_argument("--secrets", help="path to secrets.json for ${VAR} resolution")
    ap.add_argument("--enabled-only", action="store_true", help="skip options.disabled servers")
    args = ap.parse_args()

    cfg = json.load(open(args.config, encoding="utf-8"))
    secrets = {}
    if args.secrets and os.path.exists(args.secrets):
        secrets = json.load(open(args.secrets, encoding="utf-8"))

    servers = cfg.get("mcpServers", {})
    out = {}
    report = []
    for name, spec in servers.items():
        norm, disabled, warnings = normalize(name, spec, secrets)
        if disabled and args.enabled_only:
            report.append(f"SKIP (disabled): {name}")
            continue
        out[name] = norm
        flag = " [disabled in TS -> toggle off after import]" if disabled else ""
        report.append(f"{name}: {norm['type']}{flag}" + (f"  ! {'; '.join(warnings)}" if warnings else ""))

    json.dump({"mcpServers": out}, sys.stdout, indent=2)
    sys.stdout.write("\n")
    print(f"\n# {len(out)} servers normalized (of {len(servers)}):", file=sys.stderr)
    for line in report:
        print(f"#   {line}", file=sys.stderr)


if __name__ == "__main__":
    main()
