"""Verify or repair one Codex session using its native rollout; offline writes only."""

import argparse
import contextlib
import ctypes
import datetime
import json
import os
from pathlib import Path
import shutil
import tempfile

from codex_usage.repair import plan_repair
from codex_usage.artifacts import plan_artifact_repair


@contextlib.contextmanager
def offline_lock(data_dir):
    path = Path(data_dir) / "instance.lock"
    if os.name == "nt":
        from ctypes import wintypes
        api = ctypes.WinDLL("kernel32", use_last_error=True)
        api.CreateFileW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE]
        api.CreateFileW.restype = wintypes.HANDLE
        api.CloseHandle.argtypes = [wintypes.HANDLE]
        handle = api.CreateFileW(str(path), 0xC0000000, 0, None, 4, 0x80, None)
        if handle == ctypes.c_void_p(-1).value:
            raise RuntimeError("TionHarness is running or its data lock is unavailable; stop it before applying the repair")
        try:
            yield
        finally:
            api.CloseHandle(handle)
    else:
        import fcntl
        with open(path, "a+b") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            yield


def apply_plan(data_dir, files):
    root = Path(data_dir).resolve()
    backup = root / "backups" / ("codex-usage-" + datetime.datetime.now().strftime("%Y%m%d-%H%M%S-%f"))
    originals = {}
    for path, body in files.items():
        relative = path.resolve().relative_to(root)
        target = backup / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(path, target)
        originals[path] = target
    try:
        for path, body in files.items():
            with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", newline="\n", dir=path.parent, delete=False) as stream:
                stream.write(body)
                temporary = stream.name
            try:
                os.replace(temporary, path)
            finally:
                if os.path.exists(temporary):
                    os.unlink(temporary)
    except Exception:
        for path, original in originals.items():
            shutil.copy2(original, path)
        raise
    return str(backup)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-dir", required=True)
    parser.add_argument("--workspace", required=True)
    parser.add_argument("--session", required=True)
    parser.add_argument("--rollout", required=True)
    parser.add_argument("--timezone-hours", type=int, default=3)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--repair-artifacts", action="store_true", help="Recover owned text file copies from their ignored legacy inline updates")
    args = parser.parse_args()
    lock = offline_lock(args.data_dir) if args.apply else contextlib.nullcontext()
    with lock:
        files, summary = plan_repair(args.data_dir, args.workspace, args.session, args.rollout, args.timezone_hours)
        if args.repair_artifacts:
            artifact_files, recovered = plan_artifact_repair(args.data_dir, args.workspace, args.session)
            files.update(artifact_files)
            summary.update(changed=bool(files), files=len(files), recoveredArtifacts=recovered)
        if args.apply and files:
            summary["backup"] = apply_plan(args.data_dir, files)
        print(json.dumps(summary))


if __name__ == "__main__":
    main()
