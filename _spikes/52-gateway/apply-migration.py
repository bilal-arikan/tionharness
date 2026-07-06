#!/usr/bin/env python3
"""Apply the normalized import.json to TionSwarm workspaces (Doc 52 Faz 3 live migration).

For each target workspace: OVERWRITE mode = delete any existing MCP server whose name
collides with an import entry, then bulk-import all entries. Uses only urllib (no deps).

  python apply-migration.py import.json --base http://127.0.0.1:8090 [--ws WS1 WS5 ...]
Omit --ws to target every workspace the instance reports.
"""
import argparse
import json
import sys
import urllib.request


def http(method, url, ws=None, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if ws:
        req.add_header("X-Workspace-Id", ws)
    if data is not None:
        req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req, timeout=30) as resp:
        raw = resp.read().decode()
        return resp.status, (json.loads(raw) if raw.strip() else None)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("import_json")
    ap.add_argument("--base", default="http://127.0.0.1:8090")
    ap.add_argument("--ws", nargs="*", help="workspace ids; default = all")
    args = ap.parse_args()

    doc = json.load(open(args.import_json, encoding="utf-8"))
    import_names = set(doc.get("mcpServers", {}).keys())
    disable_after = set(doc.get("_disabled", []))  # servers disabled in TS -> turn off post-import
    print(f"import.json: {len(import_names)} servers ({len(disable_after)} to disable after import)")

    workspaces = args.ws
    if not workspaces:
        _, wl = http("GET", f"{args.base}/api/workspaces")
        workspaces = [w["id"] for w in (wl if isinstance(wl, list) else wl.get("workspaces", []))]
    print(f"target workspaces: {', '.join(workspaces)}\n")

    for ws in workspaces:
        print(f"=== {ws} ===")
        # 1) list existing, delete name-collisions (overwrite)
        _, existing = http("GET", f"{args.base}/api/mcp-servers", ws=ws)
        rows = existing if isinstance(existing, list) else (existing or {}).get("servers", [])
        deleted = 0
        for s in rows:
            if s.get("name") in import_names:
                try:
                    http("DELETE", f"{args.base}/api/mcp-servers/{s['id']}", ws=ws)
                    deleted += 1
                except Exception as e:
                    print(f"  ! delete {s.get('name')} failed: {e}")
        # 2) bulk import
        status, res = http("POST", f"{args.base}/api/mcp-servers/import", ws=ws, body={"mcpServers": doc["mcpServers"]})
        created = len((res or {}).get("created", []))
        errs = (res or {}).get("errors", {})
        # 3) disable the TS-disabled servers so the app does not eagerly dial backends that
        #    are not running (import always creates them enabled).
        disabled = 0
        if disable_after:
            _, rows = http("GET", f"{args.base}/api/mcp-servers", ws=ws)
            rows = rows if isinstance(rows, list) else (rows or {}).get("servers", [])
            for s in rows:
                if s.get("name") in disable_after and s.get("enabled", True):
                    http("POST", f"{args.base}/api/mcp-servers/{s['id']}/toggle", ws=ws, body={"enabled": False})
                    disabled += 1
        print(f"  overwritten(deleted): {deleted}  created: {created}  disabled: {disabled}  errors: {len(errs)}")
        for name, msg in errs.items():
            print(f"    ! {name}: {msg}")
    print("\nDONE.")


if __name__ == "__main__":
    main()
