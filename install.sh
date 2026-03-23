#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing codesnips..."

cd "$SCRIPT_DIR"

if [ ! -d ".venv" ]; then
    python3 -m venv .venv
fi

.venv/bin/pip install -q rich

INSTALL_DIR="$HOME/.local/bin"
mkdir -p "$INSTALL_DIR"

cat > "$INSTALL_DIR/snips" << 'WRAPPER'
#!/bin/bash
SCRIPT_DIR="/home/dinkum/projects/codesnips"
"$SCRIPT_DIR/.venv/bin/python" "$SCRIPT_DIR/snips.py" "$@"
WRAPPER

chmod +x "$INSTALL_DIR/snips"

echo ""
echo "✓ Installed! Add to your shell config:"
echo ""
echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
echo ""
echo "Then reload your shell or run: source ~/.bashrc (or ~/.zshrc)"
echo ""
echo "Usage:"
echo "  snips                        - Show a random snippet"
echo "  snips --run                  - Dock snippets at the top of the terminal"
echo "  snips --run --dock bottom    - Dock snippets at the bottom instead"
echo "  snips --run --dock top &     - Keep snippets visible while another CLI runs"
echo "  snips docker                 - Show snippet about docker"
echo "  snips --list                 - List all terms"
echo ""
