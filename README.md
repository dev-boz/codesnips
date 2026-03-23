# codesnips

A lightweight terminal learning tool. Get bite-sized coding term snippets while you vibe code.

## Install

```bash
cd /home/dinkum/projects/codesnips
./install.sh
```

`install.sh` now builds the Unix proxy binary locally, so `go` needs to be on your `PATH`.

Then add to your shell config (`~/.bashrc` or `~/.zshrc`):
```bash
export PATH="$HOME/.local/bin:$PATH"
```

Reload: `source ~/.bashrc`

## Usage

```bash
snips                           # Show a random snippet
snips wrap                      # Proxy your shell inside a PTY with a protected top bar
snips wrap -- codex             # Run a specific CLI inside the proxy
snips --proxy                   # Alias for snips wrap
snips --run                     # Dock snippets at the top of the terminal
snips --run --height 4          # Compact top dock for Codex/Claude style CLIs
snips --run --dock bottom       # Dock snippets at the bottom instead
snips --run -i 60 --height 7    # Change interval and dock height
snips --run --dock none         # Use the old scrolling mode
snips --stop                    # Stop any background dock
snips --stop --dock top         # Stop only the top dock
snips docker                    # Show snippet about docker
snips --list                    # List all available terms
snips --search api              # Search for terms containing 'api'
```

For the "keep this visible while I use another CLI" workflow, run it in the background:

```bash
snips --run --dock top --height 4 &
snips --run --dock bottom &
```

Proxy mode is the new preferred workflow on Linux/macOS:

```bash
snips wrap
snips wrap --height 4 --interval 45
snips wrap -- claude
snips wrap -- vim
```

`snips wrap` starts your shell or target command inside a PTY and keeps the snippet bar pinned at the top by rewriting absolute cursor/scroll-region VT sequences before they hit your terminal. That avoids the old “two writers fighting over one screen” failure mode, so full-screen apps like Codex, Claude, `vim`, `less`, and `top` can stay inside the lowered viewport.

`snips --run` remains available as a simpler fallback for split panes, background docks, or cases where you do not want a proxy shell.

Best experience on Windows is still a split pane for now. The proxy architecture is being set up so the ANSI rewriting logic can be reused with ConPTY in a later Windows phase.

## Add Your Own Snippets

Edit `snippets.json` to add more terms:

```json
{
  "myterm": {
    "term": "My Term",
    "definition": "A clear, concise explanation."
  }
}
```

## Tips

- Use `--dock top` or `--dock bottom` to reserve a fixed area of the terminal
- Prefer `snips wrap` when you want to run Claude/Codex/vim in the same terminal without output corruption
- For Codex specifically, prefer `--dock top --height 4`; it uses the bottom of the screen itself
- Run `snips --run ... &` before starting Claude, Codex, or another full-screen CLI
- Use `snips --stop` to kill the background dock after your coding session
- Run `snips --stop` from the same terminal session to collapse a top dock cleanly
- Use `--dock none --no-clear` if you want the old scrolling history behavior
- Create a custom snippets file: `snips -f my-snippets.json`
