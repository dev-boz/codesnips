# codesnips

A lightweight terminal learning tool with a Go-only CLI.

## Install

```bash
go install github.com/dev-boz/codesnips/cmd/snips@latest
```

Requires Go 1.25+.

Ensure your Go bin directory is on your `PATH`:

```bash
export PATH="$HOME/go/bin:$PATH"
```

## Usage

```bash
snips                           # Show a random snippet
snips docker                    # Show a specific term
snips --list                    # List all available terms
snips --search api              # Search snippets
snips wrap                      # Wrap your shell with the snippet bar proxy
snips wrap -- codex             # Run a specific command inside the proxy
snips wrap --height 3 --interval 45
```

Use a custom snippets file:

```bash
snips --file ./snippets.json
snips wrap --file ./snippets.json
```

If `--file` is omitted, the CLI loads `./snippets.json` when present, otherwise it falls back to built-in snippets bundled in the binary.

## Proxy mode

`snips wrap` runs your shell or command inside a PTY and keeps the snippets bar pinned at the bottom. The proxy rewrites absolute cursor and scroll-region VT sequences so full-screen terminal apps (for example `vim`, `less`, and CLI agents) continue to render correctly.

## Notes

- `snips --run` compatibility mode has been removed; use `snips wrap`.
- The project no longer depends on Python, shell wrapper generation, or CGO.
