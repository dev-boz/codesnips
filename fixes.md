# codesnips — Improvement Plan

Distilled from reviews by 5 AI tools (mini, gemini, sonnet, cursor, gpt). Noise removed, deduplicated, ranked by impact.

---

## Real Bug

- [x] **UTF-8 cursor desync** — `handleGroundByte` in `main.go` increments cursor position by 1 per byte, not per character. Multi-byte UTF-8 will break the ANSI rewriter's cursor tracking. (Found by: gpt)

---

## Phase 1 — Fix what's broken and embarrassing

- [x] Fix the UTF-8 cursor tracking bug (see above)
- [x] Update `.gitignore` — add `.venv/`, `__pycache__/`, built binaries, temp files. Remove any already-committed artifacts from tracking.
- [x] Fix `go.mod` module path to `github.com/<you>/codesnips` so `go install` works properly

## Phase 2 — Structural refactor

- [x] Replace CGO/inline C with `creack/pty` (`github.com/creack/pty`) — eliminates gcc dependency, restores Go cross-compilation
- [x] Split `main.go` into packages: `internal/pty`, `internal/ansi`, `internal/snippets`, etc. The file is 750-1200+ lines with ANSI rewriting, PTY management, snippet storage, terminal modes, and the main event loop all crammed together.
- [x] Delete hand-rolled `min`/`max` functions — use Go 1.21 builtins
- [x] Replace `containsString` linear scan with `map[string]struct{}` for the `recent` set in snippet rotation

## Phase 3 — Testing & CI

- [x] Table-driven golden tests for the ANSI rewriter — feed sample byte streams, assert rewritten output snapshots. This is the most complex and most breakable component with zero tests today.
- [x] Unit tests for snippet rotation, CSI parsing, clamping logic
- [x] GitHub Actions CI: `go vet`, `go test ./...`, build gate on PRs

## Phase 4 — Simplify the language story (Go-only single binary)

The current stack is Python + Go + inline C + Bash. Two reviewers independently recommended consolidating to Go-only.

- [x] Port Python CLI logic to Go — it's just JSON loading, flag parsing, random selection, listing, searching. Use `lipgloss` (github.com/charmbracelet/lipgloss) to replace Python's `rich` for styled terminal output.
- [x] Kill `install.sh` and the generated bash wrapper — `install.sh` generates a wrapper that bypasses most of the Python dispatch logic anyway, making it dead code.
- [x] Kill `_legacy_run_to_proxy_args` — supports flags that are documented as removed. Just drop it.
- [x] Ship a single `go install`-able binary. No more requiring Python + Go + gcc on the user's machine.

---

## What was dismissed as noise

These came up in reviews but are either generic OSS checklist items or low-leverage for where the project is right now:

- Semantic commits / CHANGELOG / automated releases (mini) — premature for current stage
- Vulnerability scanning (mini) — not relevant for a local terminal tool
- LICENSE / CONTRIBUTING.md / SECURITY.md (gpt) — do these when you actually publish, not before
- Structured logging / debug mode (cursor) — nice to have, not critical
- Pin Python dependencies with lock file (cursor) — moot if Phase 4 removes Python entirely

---

## Suggested order (fastest ROI)

- **Week 1**: Phase 1 (hygiene + bug fix — quick wins, unblocks everything)
- **Week 2**: Phase 2 (structural refactor — biggest maintainability improvement)
- **Week 3**: Phase 3 (tests + CI — safety net before the big rewrite)
- **Week 4+**: Phase 4 (Go-only consolidation — the end goal)
