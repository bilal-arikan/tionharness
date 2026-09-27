"""Label known fixture commands conservatively; unknown commands need review."""
import hashlib
import json
import re
import sys
from pathlib import Path
data = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
for row in data["results"]:
    seen = set()
    for step in row["trace"]:
        command = step["input"]
        try:
            value = json.loads(command)
            if isinstance(value, str):
                command = value
        except ValueError:
            pass
        step["commandText"] = command
        purposes = []
        if step["tool"] == "apply_patch":
            purposes = ["düzenleme"]
        elif step["tool"] == "todo_list":
            purposes = ["plan"]
        elif step["tool"] == "shell":
            if re.search(r"Set-Content|WriteAllText|write_text|write_bytes|Out-File", command, re.I):
                purposes = ["incelenmedi"]
            else:
                if re.search(r"Get-ChildItem|rg --files", command, re.I):
                    purposes.append("keşif")
                if re.search(r"Get-Content", command, re.I):
                    purposes.append("okuma")
                if re.search(r"\bassert\b|unittest|pytest|py_compile", command):
                    purposes.append("doğrulama")
                if re.search(r"Remove-Item", command, re.I):
                    purposes.append("temizlik")
        step["purposes"] = purposes or ["incelenmedi"]
        step["purpose"] = "+".join(step["purposes"])
        summaries = {"keşif":"Klasör/dosya listeleme", "okuma":"solution.py okuma", "düzenleme":"Dosya yaması", "doğrulama":"Yerel Python kontrolleri", "temizlik":"Üretilen Python cache dosyalarını temizleme"}
        step["summary"] = " + ".join(summaries.get(p,p) for p in step["purposes"])
        key = (step["tool"], command)
        # Empty patch payloads cannot establish repeated calls.
        step["identicalInputSeenEarlier"] = bool(command) and key in seen
        seen.add(key)
data["fixturesSHA256"] = {
    p.name: hashlib.sha256(p.read_bytes()).hexdigest()
    for p in sorted(Path("scripts/codexbench/testdata").iterdir()) if p.is_file()
}
Path(sys.argv[2]).write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
