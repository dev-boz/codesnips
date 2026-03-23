#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing codesnips..."

cd "$SCRIPT_DIR"

if ! command -v go >/dev/null 2>&1; then
    echo "Go is required to build snips proxy mode."
    exit 1
fi

if [ ! -d ".venv" ]; then
    python3 -m venv .venv
fi

.venv/bin/pip install -q rich

INSTALL_DIR="$HOME/.local/bin"
mkdir -p "$INSTALL_DIR"
go build -o "$INSTALL_DIR/snips-proxy" ./cmd/snips-proxy

cat > "$INSTALL_DIR/snips" << WRAPPER
#!/bin/bash
set -e

if [ "\$#" -gt 0 ] && [ "\$1" = "wrap" ]; then
    shift
    exec "$INSTALL_DIR/snips-proxy" --snippets-file "$SCRIPT_DIR/snippets.json" "\$@"
fi

if [ "\$#" -gt 0 ] && [ "\$1" = "--proxy" ]; then
    shift
    exec "$INSTALL_DIR/snips-proxy" --snippets-file "$SCRIPT_DIR/snippets.json" "\$@"
fi

exec "$SCRIPT_DIR/.venv/bin/python" "$SCRIPT_DIR/snips.py" "\$@"
WRAPPER

chmod +x "$INSTALL_DIR/snips"
chmod +x "$INSTALL_DIR/snips-proxy"

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
echo "  snips wrap                   - Run a proxy shell with a protected top bar"
echo "  snips --run --dock bottom    - Dock snippets at the bottom instead"
echo "  snips --run --dock top &     - Keep snippets visible while another CLI runs"
echo "  snips docker                 - Show snippet about docker"
echo "  snips --list                 - List all terms"
echo ""
