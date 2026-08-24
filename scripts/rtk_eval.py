"""Measure whether rtk earns a place in TionHarness's rewrite allowlist, per command.

For each command it runs the ORIGINAL and the rtk-REWRITTEN form, then counts
tokens with tiktoken (o200k_base) — a real tokenizer, applied identically to every
variant. sqz's own stderr counter was tried first and rejected: sqz self-gates and
reports nothing for small or incompressible output, so half the table came back
unmeasured and the two halves would not have been comparable anyway.

Columns: raw | sqz(raw) | rtk | sqz(rtk). The verdict compares the best
rtk-inclusive result against the best rtk-free one — rtk is only worth wiring if it
beats what sqz already achieves on its own.

Usage: python rtk_eval.py <spec-file>
  spec lines: <label>\t<cwd>\t<command>
"""
import os
import re
import subprocess
import sys

import tiktoken

ENC = tiktoken.get_encoding("o200k_base")
# A run-unique salt defeats sqz's persistent dedup cache, which would otherwise
# collapse a repeated payload to a "§ref:" handle and flatter sqz unfairly.
SALT = os.urandom(6).hex()
_PROBE_N = 0
# Git-bash resets PATH when it runs the login profile, so `bash -lc` loses the
# Windows PATH additions and every toolchain reports "command not found" — which
# silently measured 13-token error strings as if they were command output. Hence
# `bash -c` (no profile) plus an explicit toolchain list.
#
# The list is injected via the child's ENVIRONMENT, not an `export PATH="…"` shell
# prefix: Python's Windows argument quoting backslash-escapes the inner double
# quotes, so bash received a literal \" and the assignment silently produced a
# broken PATH. Passing env= sidesteps command-line quoting entirely.
TOOLCHAINS = [
    r"C:\Python313", r"C:\Python313\Scripts",
    r"C:\Users\user\.cargo\bin",
    r"C:\Program Files\nodejs",
    r"C:\Users\user\AppData\Local\Programs\@external-agentelectron\resources\app\resources\bin\win32-x64",
]
CHILD_ENV = dict(os.environ, PATH=";".join(TOOLCHAINS) + ";" + os.environ.get("PATH", ""))

# `bash` on PATH resolves to C:\Windows\System32\bash.exe — the WSL LAUNCHER, not
# git-bash. That is a different OS: Windows toolchains are not on its PATH, drives
# live under /mnt/c, and it double-expands `-c` payloads (a `$PATH` in the command
# was substituted by the outer shell before bash ever saw it, producing a syntax
# error). TionHarness refuses this binary for the same reasons — see
# isWSLBashLauncher in internal/tools/builtin_shell_posix.go. Resolve git-bash the
# way the product does, so the measurements run in the shell the agent gets.
def _git_bash() -> str:
    for c in (r"C:\Program Files\Git\bin\bash.exe", r"C:\Program Files\Git\usr\bin\bash.exe",
              r"C:\Program Files (x86)\Git\bin\bash.exe"):
        if os.path.isfile(c):
            return c
    raise SystemExit("git-bash bulunamadi; WSL bash ile olcum yapilmaz (farkli OS)")


BASH = _git_bash()
# Below this, compression is not the question being asked: rtk exists for commands
# that flood the context, and a 60-token result is noise either way.
INTERESTING_MIN_TOKENS = 300


def count(text: bytes) -> int:
    return len(ENC.encode(text.decode("utf-8", "replace"), disallowed_special=()))


def winpath(p: str) -> str:
    """Git-bash spells drives as /c/...; Windows CreateProcess cwd needs C:\\..."""
    m = re.fullmatch(r"/([a-zA-Z])/(.*)", p)
    return f"{m.group(1).upper()}:\\" + m.group(2).replace("/", "\\") if m else p


def run(cmd: str, cwd: str) -> bytes:
    """Run cmd through bash, capturing combined output (what the shell tool returns)."""
    try:
        p = subprocess.run([BASH, "-c", cmd], cwd=winpath(cwd), env=CHILD_ENV,
                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=900)
        out = p.stdout
        # A missing toolchain measures as a tiny "command not found" string that
        # would silently look like brilliant compression. Fail loudly instead.
        if b"command not found" in out and len(out) < 200:
            raise SystemExit(f"TOOLCHAIN EKSIK: {cmd!r} -> {out.decode('utf-8', 'replace').strip()}")
        return out
    except subprocess.TimeoutExpired:
        return b"[timeout]"


def sqz(text: bytes, label: str) -> bytes:
    """Compress text the way the shell output filter would. Fails open.

    Defeating sqz's dedup cache is essential and takes BOTH measures below.

    A unique salt line alone is not enough: sqz dedups per content block, so the
    bulk of two near-identical payloads — a command's raw output and rtk's
    near-passthrough of that same output — still collapses to a "§ref:" handle on
    whichever is measured second. That reads as a spectacular rtk win and is pure
    artifact; it inflated an early `npm run build` reading to a fictitious 54%
    saving, and a `pip list` reading to 12 tokens. So the cache is also cleared
    before every single measurement.
    """
    if not text.strip():
        return text
    global _PROBE_N
    _PROBE_N += 1
    subprocess.run(["sqz", "reset", "--cache-only", "-y"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    salted = f"# probe {SALT} {label} #{_PROBE_N}\n".encode() + text
    p = subprocess.run(["sqz", "compress", "--cmd", label], input=salted,
                       stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    out = p.stdout
    return out if out.strip() else salted


def rewrite(cmd: str):
    """rtk's own rewrite for cmd, or None. The exit code is ignored on purpose: rtk
    exits 3 on success despite documenting 0, so the OUTPUT is what we trust."""
    p = subprocess.run(["rtk", "rewrite", cmd], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    out = p.stdout.decode("utf-8", "replace").strip()
    return out if out.startswith("rtk ") else None


def verdict(best_without: int, best_with: int, raw: int) -> str:
    if raw < INTERESTING_MIN_TOKENS:
        return f"onemsiz (ham {raw} tok)"
    if best_with < best_without * 0.9:
        return f"EKLE   (%{round((1 - best_with / best_without) * 100)} daha iyi)"
    if best_with > best_without * 1.1:
        return f"EKLEME (%{round((best_with / best_without - 1) * 100)} daha kotu)"
    return "notr (~esit)"


def main():
    print(f"{'komut':<28}{'ham':>7}{'sqz':>7}{'rtk':>7}{'rtk+sqz':>9}  karar")
    print("-" * 82)
    for line in open(sys.argv[1], encoding="utf-8"):
        line = line.rstrip("\n")
        if not line.strip() or line.startswith("#"):
            continue
        label, cwd, cmd = line.split("\t", 2)

        rw = rewrite(cmd)
        if rw is None:
            print(f"{label:<28}{'-':>7}{'-':>7}{'-':>7}{'-':>9}  rtk desteklemiyor")
            continue

        raw_out = run(cmd, cwd)
        rtk_out = run(rw, cwd)
        raw, raw_sqz = count(raw_out), count(sqz(raw_out, "plain"))
        rtk, rtk_sqz = count(rtk_out), count(sqz(rtk_out, "rtk"))

        best_without, best_with = min(raw, raw_sqz), min(rtk, rtk_sqz)
        v = verdict(best_without, best_with, raw) if best_without else "olculemedi"
        print(f"{label:<28}{raw:>7}{raw_sqz:>7}{rtk:>7}{rtk_sqz:>9}  {v}")


if __name__ == "__main__":
    main()
