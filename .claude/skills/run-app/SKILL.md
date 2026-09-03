---
name: run-app
description: >
  Start TionHarness locally and open it in the browser to verify a change in the real app
  (not only tests). Use for "/run-app", "uygulamayı başlat", "çalıştır ve bak", or when a
  UI/API change needs a live check. Uses .claude/launch.json.
---

# Run the app

Two launch configurations exist in `.claude/launch.json`:

| Name | What | URL |
|---|---|---|
| `tionharness` | Go server (`go run ./cmd/tionharness`), serves the embedded frontend | http://127.0.0.1:8080 |
| `frontend-dev` | Vite dev server for the React frontend (hot reload) | http://127.0.0.1:5173 |

Steps:

1. **Backend only / API check:** `preview_start` with name `tionharness`. The embedded UI
   comes from `internal/web/dist`; if it is missing or stale, build it first:
   `cd frontend && npm run build` (output lands in `internal/web/dist`).
2. **Frontend iteration:** start `tionharness` AND `frontend-dev`; open the Vite URL, it
   proxies `/api` to the Go server.
3. Data dir defaults to `~/.tionharness` (`TIONHARNESS_DATA_DIR` overrides). The user's
   own instance may already hold `instance.lock` there — if the server refuses to start
   because another instance is live, do NOT kill it; ask the user or use a scratch data
   dir: `TIONHARNESS_DATA_DIR=<scratchpad>/th-data`.
4. The listen address is `TIONHARNESS_ADDR` (default `127.0.0.1:8080`). The user's daily
   instance runs on 8099; never assume it is free.
5. Verify with `read_page`/`get_page_text` on the relevant screen, or `curl` an endpoint
   (bearer auth is opt-in and off by default). Read server logs with `preview_logs`.

Stop servers you started (`preview_stop`) before finishing.
