"""Recover file artifacts whose inline updates never reached the served copy."""

import json
from pathlib import Path


def plan_artifact_repair(data_dir, workspace, session_id):
    root = Path(data_dir) / "workspaces" / workspace
    sandbox = (root / "workspace").resolve()
    rows = [(path, json.loads(path.read_text(encoding="utf-8"))) for path in (root / "store/artifacts").glob("*.json")]
    files = {}
    recovered = []
    for path, row in rows:
        if row.get("sessionId") != session_id or row.get("kind") != "file" or not row.get("content"):
            continue
        source = row.get("sourcePath", "")
        if Path(source).suffix.lower() not in (".md", ".txt", ".html"):
            raise ValueError("File artifact has an unverified inline format")
        target = (sandbox / source).resolve()
        # Only the session's owned, unshared copies may be overwritten.
        target.relative_to(sandbox / "artifacts" / session_id)
        if sum(other.get("sourcePath") == source or other.get("contentFile") == source for _, other in rows) != 1:
            raise ValueError("Artifact source is shared; refusing to overwrite another artifact")
        target.read_bytes()  # Missing sources must fail before any write.
        files[target] = row["content"]
        row["content"] = ""
        files[path] = json.dumps(row, ensure_ascii=False, indent=2) + "\n"
        recovered.append(row["id"])
    return files, recovered
