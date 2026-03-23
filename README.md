# codesnips

A lightweight terminal learning tool. Get bite-sized coding term snippets while you vibe code.

## Install

```bash
cd /home/dinkum/projects/codesnips
./install.sh
```

Then add to your shell config (`~/.bashrc` or `~/.zshrc`):
```bash
export PATH="$HOME/.local/bin:$PATH"
```

Reload: `source ~/.bashrc`

## Usage

```bash
snips                           # Show a random snippet
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
- For Codex specifically, prefer `--dock top --height 4`; it uses the bottom of the screen itself
- Run `snips --run ... &` before starting Claude, Codex, or another full-screen CLI
- Use `snips --stop` to kill the background dock after your coding session
- Run `snips --stop` from the same terminal session to collapse a top dock cleanly
- Use `--dock none --no-clear` if you want the old scrolling history behavior
- Create a custom snippets file: `snips -f my-snippets.json`
