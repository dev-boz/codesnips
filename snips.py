#!/usr/bin/env python3
"""
codesnips - A lightweight terminal learning tool
Displays coding term snippets in a fixed terminal region or as a one-off panel
"""

import argparse
import io
import json
import os
import random
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Optional
from typing import Sequence

try:
    from rich.console import Console
    from rich.panel import Panel
    from rich.text import Text
except ImportError:
    sys.stderr.write(
        "Missing dependency 'rich'. Install it with:\n"
        "  ./install.sh   (creates venv + installs deps)\n"
        "  pip install rich\n"
    )
    sys.exit(2)


class TerminalDock:
    SAVE_CURSOR = "\x1b7"
    RESTORE_CURSOR = "\x1b8"
    CLEAR_LINE = "\x1b[2K"
    RESET_SCROLL_REGION = "\x1b[r"

    def __init__(self, position: str, height: int):
        self.position = position
        self.requested_height = max(1, height)
        self.active = False
        self.rows = 0
        self.width = 0
        self.height = 0
        self.dock_rows: list[int] = []
        self.scroll_top = 1
        self.scroll_bottom = 1
        self.broken = False

    def _write(self, text: str):
        if self.broken:
            raise OSError("terminal dock is unavailable")

        try:
            sys.stdout.write(text)
            sys.stdout.flush()
        except (BrokenPipeError, OSError):
            self.broken = True
            raise

    def _compute_layout(self):
        size = shutil.get_terminal_size((80, 24))
        rows = max(2, size.lines)
        width = max(20, size.columns)
        height = min(self.requested_height, rows - 1)
        if height < 1:
            return None

        if self.position == "top":
            dock_rows = list(range(1, height + 1))
            scroll_top = height + 1
            scroll_bottom = rows
        else:
            dock_rows = list(range(rows - height + 1, rows + 1))
            scroll_top = 1
            scroll_bottom = rows - height

        if scroll_top > scroll_bottom:
            return None

        return {
            "rows": rows,
            "width": width,
            "height": height,
            "dock_rows": dock_rows,
            "scroll_top": scroll_top,
            "scroll_bottom": scroll_bottom,
        }

    def activate(self, force: bool = False) -> bool:
        if self.broken or self.position == "none" or not sys.stdout.isatty():
            self.active = False
            return False

        layout = self._compute_layout()
        if layout is None:
            self.active = False
            return False

        previous_rows = list(self.dock_rows)
        changed = force or not self.active or any(
            getattr(self, key) != value
            for key, value in (
                ("rows", layout["rows"]),
                ("width", layout["width"]),
                ("height", layout["height"]),
                ("dock_rows", layout["dock_rows"]),
                ("scroll_top", layout["scroll_top"]),
                ("scroll_bottom", layout["scroll_bottom"]),
            )
        )

        self.rows = layout["rows"]
        self.width = layout["width"]
        self.height = layout["height"]
        self.dock_rows = layout["dock_rows"]
        self.scroll_top = layout["scroll_top"]
        self.scroll_bottom = layout["scroll_bottom"]

        if changed:
            chunks = [self.SAVE_CURSOR, self.RESET_SCROLL_REGION]
            for row in previous_rows:
                chunks.append(f"\x1b[{row};1H{self.CLEAR_LINE}")
            chunks.append(f"\x1b[{self.scroll_top};{self.scroll_bottom}r")
            chunks.append(self.RESTORE_CURSOR)
            self._write("".join(chunks))

        self.active = True
        return True

    def render(self, lines: list[str]):
        if self.broken or not self.active:
            return

        padded_lines = lines[: self.height] + [""] * max(0, self.height - len(lines))
        chunks = [self.SAVE_CURSOR]
        for row, line in zip(self.dock_rows, padded_lines):
            chunks.append(f"\x1b[{row};1H{self.CLEAR_LINE}{line}")
        chunks.append(self.RESTORE_CURSOR)
        self._write("".join(chunks))

    def cleanup(self):
        if self.broken:
            self.best_effort_reset()
            return

        if not self.active:
            return

        chunks = [self.SAVE_CURSOR, self.RESET_SCROLL_REGION]
        for row in self.dock_rows:
            chunks.append(f"\x1b[{row};1H{self.CLEAR_LINE}")
        chunks.append(self.RESTORE_CURSOR)
        self._write("".join(chunks))
        self.active = False

    def best_effort_reset(self):
        try:
            sys.stdout.write(self.RESET_SCROLL_REGION)
            sys.stdout.flush()
        except (BrokenPipeError, OSError):
            pass
        self.active = False


class CodeSnips:
    def __init__(self, snippets_file: Optional[str] = None):
        self.console = Console()

        if snippets_file:
            self.snippets_path = Path(snippets_file)
        else:
            self.snippets_path = Path(__file__).parent / "snippets.json"

        self.snippets = self._load_snippets()
        self.shown_recently = []
        self.max_recent = 10
        self.stop_requested = False
        self.resize_requested = False
        self.last_render_state = None
        self._previous_signal_handlers = {}
        self.pid_file: Optional[Path] = None
        self.pause_on: list[str] = ["claude"]
        self.pause_enabled = True

    def _load_snippets(self) -> dict:
        if not self.snippets_path.exists():
            print(f"Snippets file not found: {self.snippets_path}")
            sys.exit(1)

        with open(self.snippets_path, "r", encoding="utf-8") as f:
            return json.load(f)

    def _state_dir(self) -> Path:
        xdg_state_home = os.environ.get("XDG_STATE_HOME")
        if xdg_state_home:
            return Path(xdg_state_home) / "codesnips"
        return Path.home() / ".local" / "state" / "codesnips"

    def _pid_file_for(self, dock: str) -> Path:
        suffix = dock if dock in {"top", "bottom"} else "run"
        return self._state_dir() / f"{suffix}.pid"

    def _current_tty(self) -> Optional[str]:
        try:
            return os.ttyname(sys.stdout.fileno())
        except OSError:
            return None

    def _foreground_cmdline(self) -> Optional[str]:
        """Best-effort foreground process cmdline for this terminal.

        Uses tcgetpgrp() and assumes the process group leader PID matches PGID,
        which is true for typical shells and CLIs. Never reads stdin.
        """
        try:
            fd = sys.stdout.fileno()
            fg_pgrp = os.tcgetpgrp(fd)
        except OSError:
            return None

        if fg_pgrp <= 0:
            return None

        cmdline_path = Path(f"/proc/{fg_pgrp}/cmdline")
        try:
            if not cmdline_path.exists():
                return None
            raw = cmdline_path.read_bytes()
        except OSError:
            return None

        if not raw:
            return None

        # /proc/<pid>/cmdline is NUL-separated.
        cmdline = raw.decode("utf-8", errors="replace").replace("\x00", " ").strip()
        return cmdline or None

    def _should_pause_dock(self) -> bool:
        if not self.pause_enabled or not self.pause_on:
            return False

        cmdline = self._foreground_cmdline()
        if not cmdline:
            return False

        lower = cmdline.lower()
        for token in self.pause_on:
            if token and token.lower() in lower:
                return True
        return False

    def _read_state(self, pid_file: Path) -> Optional[dict]:
        try:
            pid_text = pid_file.read_text(encoding="utf-8").strip()
        except FileNotFoundError:
            return None
        except OSError:
            return None

        if not pid_text:
            return None

        if pid_text.isdigit():
            return {"pid": int(pid_text)}

        try:
            state = json.loads(pid_text)
        except json.JSONDecodeError:
            return None

        if not isinstance(state, dict):
            return None
        return state

    def _read_pid(self, pid_file: Path) -> Optional[int]:
        state = self._read_state(pid_file)
        if state is None:
            return None

        pid = state.get("pid")
        if isinstance(pid, int):
            return pid
        if isinstance(pid, str) and pid.isdigit():
            return int(pid)

        return None

    def _wait_for_pid_exit(self, pid: int, timeout: float = 2.0) -> bool:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if not self._pid_is_running(pid):
                return True
            time.sleep(0.05)
        return not self._pid_is_running(pid)

    def _collapse_terminal_space(self, state: dict):
        tty_path = state.get("tty")
        if tty_path is None or tty_path != self._current_tty():
            return

        dock = state.get("dock")
        height = state.get("height")
        if dock != "top" or not isinstance(height, int) or height < 1:
            return

        try:
            with open(tty_path, "w", encoding="utf-8", buffering=1) as tty:
                tty.write(f"{TerminalDock.RESET_SCROLL_REGION}\x1b[{height}S")
                tty.flush()
        except OSError:
            pass

    def _register_pid_file(self, dock: str, height: int):
        run_state = {
            "pid": os.getpid(),
            "dock": dock,
            "height": max(1, height),
            "tty": self._current_tty(),
        }
        pid_file = self._pid_file_for(dock)
        pid_file.parent.mkdir(parents=True, exist_ok=True)
        pid_file.write_text(json.dumps(run_state), encoding="utf-8")
        self.pid_file = pid_file

    def _pid_is_running(self, pid: int) -> bool:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            return False
        except PermissionError:
            return True
        return True

    def _unregister_pid_file(self):
        if self.pid_file is None:
            return

        try:
            state = self._read_state(self.pid_file)
            recorded_pid = None if state is None else state.get("pid")
            if recorded_pid == os.getpid():
                self.pid_file.unlink(missing_ok=True)
        except OSError:
            pass
        finally:
            self.pid_file = None

    def stop(self, dock: Optional[str] = None):
        if dock and dock != "none":
            pid_files = [self._pid_file_for(dock)]
        else:
            pid_files = [self._pid_file_for("top"), self._pid_file_for("bottom"), self._pid_file_for("none")]

        stopped = []
        missing = []

        for pid_file in pid_files:
            state = self._read_state(pid_file)
            pid = None if state is None else state.get("pid")
            if pid is None:
                missing.append(pid_file)
                continue

            if not self._pid_is_running(pid):
                try:
                    pid_file.unlink(missing_ok=True)
                except OSError:
                    pass
                missing.append(pid_file)
                continue

            try:
                try:
                    cmdline_path = Path(f"/proc/{pid}/cmdline")
                    if cmdline_path.exists():
                        cmdline = cmdline_path.read_bytes().decode(
                            "utf-8", errors="replace"
                        )
                        if "snips" not in cmdline:
                            self.console.print(
                                f"[yellow]PID {pid} is not a snips process (stale PID file). Cleaning up.[/yellow]"
                            )
                            pid_file.unlink(missing_ok=True)
                            missing.append(pid_file)
                            continue
                except OSError:
                    pass

                os.kill(pid, signal.SIGTERM)
                if self._wait_for_pid_exit(pid):
                    self._collapse_terminal_space(state)
                stopped.append(pid)
            except ProcessLookupError:
                try:
                    pid_file.unlink(missing_ok=True)
                except OSError:
                    pass
                missing.append(pid_file)
            except OSError as exc:
                self.console.print(f"[red]Failed to stop PID {pid}: {exc}[/red]")

        if stopped:
            self.console.print(
                f"[green]Stopped codesnips process(es): {', '.join(str(pid) for pid in stopped)}[/green]"
            )
        else:
            self.console.print("[yellow]No running docked codesnips process found.[/yellow]")

    def _get_random_snippet(self) -> tuple:
        available = [k for k in self.snippets.keys() if k not in self.shown_recently]

        if not available:
            self.shown_recently = []
            available = list(self.snippets.keys())

        key = random.choice(available)
        self.shown_recently.append(key)

        if len(self.shown_recently) > self.max_recent:
            self.shown_recently.pop(0)

        return key, self.snippets[key]

    def _build_panel(self, term: str, definition: str, height: Optional[int] = None):
        header = Text("CodeSnips", style="bold yellow") + Text(
            f" {term}", style="bold cyan"
        )
        content = Text(definition, style="white")
        panel_kwargs = {
            "title": header,
            "border_style": "dim",
            "padding": (0, 1),
            "expand": True,
        }
        if height is not None:
            panel_kwargs["height"] = max(3, height)
        return Panel(content, **panel_kwargs)

    def _resolve_snippet(self, term_key: Optional[str] = None):
        if term_key:
            if term_key not in self.snippets:
                self.console.print(f"[red]Unknown term: {term_key}[/red]")
                self.console.print(
                    f"[dim]Available terms: {', '.join(sorted(self.snippets.keys())[:10])}...[/dim]"
                )
                return None
            snippet = self.snippets[term_key]
        else:
            term_key, snippet = self._get_random_snippet()

        term = snippet.get("term", term_key)
        definition = snippet.get("definition", "No definition available")
        return term, definition

    def display_snippet(self, term_key: Optional[str] = None):
        snippet_data = self._resolve_snippet(term_key)
        if snippet_data is None:
            return

        term, definition = snippet_data
        panel = self._build_panel(term, definition)

        self.console.print()
        self.console.print(panel)
        self.console.print()

    def _render_docked_lines(
        self, term: str, definition: str, interval: int, width: int, height: int
    ) -> list[str]:
        if height < 2:
            return [""] * max(1, height)

        panel_height = max(3, height - 1)
        panel = self._build_panel(term, definition, height=panel_height)
        footer = Text(
            f"refreshes every {interval}s | Ctrl+C to stop | launch another CLI below or above this dock",
            style="dim",
        )

        buffer = io.StringIO()
        render_console = Console(
            file=buffer,
            force_terminal=True,
            color_system=self.console.color_system or "standard",
            width=width,
            highlight=False,
        )
        render_console.print(panel)
        render_console.print(footer, overflow="ellipsis", no_wrap=True, crop=True)

        lines = buffer.getvalue().rstrip("\n").splitlines()
        if len(lines) < height:
            lines.extend([""] * (height - len(lines)))
        return lines[:height]

    def _handle_stop_signal(self, signum, frame):
        self.stop_requested = True

    def _handle_resize_signal(self, signum, frame):
        self.resize_requested = True

    def _install_signal_handlers(self):
        for sig in (signal.SIGTERM, signal.SIGINT):
            self._previous_signal_handlers[sig] = signal.getsignal(sig)
            signal.signal(sig, self._handle_stop_signal)

        if hasattr(signal, "SIGHUP"):
            self._previous_signal_handlers[signal.SIGHUP] = signal.getsignal(
                signal.SIGHUP
            )
            signal.signal(signal.SIGHUP, self._handle_stop_signal)

        if hasattr(signal, "SIGWINCH"):
            self._previous_signal_handlers[signal.SIGWINCH] = signal.getsignal(
                signal.SIGWINCH
            )
            signal.signal(signal.SIGWINCH, self._handle_resize_signal)

    def _restore_signal_handlers(self):
        for sig, handler in self._previous_signal_handlers.items():
            signal.signal(sig, handler)
        self._previous_signal_handlers = {}

    def run(
        self,
        interval: int = 30,
        clear: bool = True,
        dock: str = "top",
        height: int = 6,
        pause_on: Optional[Sequence[str]] = None,
        pause_enabled: bool = True,
    ):
        interval = max(1, interval)
        self.stop_requested = False
        self.resize_requested = False
        self.last_render_state = None
        self.pause_enabled = pause_enabled
        if pause_on is not None:
            self.pause_on = [t for t in pause_on if t]

        terminal_dock = TerminalDock(dock, height)
        dock_active = terminal_dock.activate()
        if dock != "none" and not dock_active:
            self.console.print(
                "[yellow]Docked mode needs a real TTY and at least 2 rows. Falling back to scrolling output.[/yellow]"
            )

        self._register_pid_file(dock, height)

        self._install_signal_handlers()

        try:
            while not self.stop_requested:
                # If an "unsafe" foreground app is running (like Claude),
                # pause refreshes so we don't corrupt its terminal UI/output.
                if dock_active and self._should_pause_dock():
                    time.sleep(0.5)
                    continue

                snippet_data = self._resolve_snippet()
                if snippet_data is None:
                    return

                term, definition = snippet_data
                self.last_render_state = (term, definition, interval)

                if dock_active:
                    try:
                        terminal_dock.render(
                            self._render_docked_lines(
                                term,
                                definition,
                                interval,
                                terminal_dock.width,
                                terminal_dock.height,
                            )
                        )
                    except (BrokenPipeError, OSError):
                        dock_active = False
                        break
                else:
                    if clear:
                        self.console.clear()
                    panel = self._build_panel(term, definition)
                    self.console.print()
                    self.console.print(panel)
                    self.console.print()

                    footer = Text()
                    footer.append("Refreshes every ", style="dim")
                    footer.append(f"{interval}s", style="bold dim")
                    footer.append(" | ", style="dim")
                    footer.append("Ctrl+C to exit", style="dim")
                    self.console.print(footer)

                deadline = time.monotonic() + interval
                while time.monotonic() < deadline and not self.stop_requested:
                    if dock_active and self._should_pause_dock():
                        time.sleep(0.5)
                        continue
                    if dock_active and self.resize_requested:
                        self.resize_requested = False
                        try:
                            dock_active = terminal_dock.activate(force=True)
                        except (BrokenPipeError, OSError):
                            dock_active = False
                            break

                        if not dock_active:
                            break

                        if self.last_render_state is not None:
                            last_term, last_definition, last_interval = self.last_render_state
                            try:
                                terminal_dock.render(
                                    self._render_docked_lines(
                                        last_term,
                                        last_definition,
                                        last_interval,
                                        terminal_dock.width,
                                        terminal_dock.height,
                                    )
                                )
                            except (BrokenPipeError, OSError):
                                dock_active = False
                                break
                    time.sleep(0.2)
        except KeyboardInterrupt:
            pass
        except (BrokenPipeError, OSError):
            pass
        except Exception:
            if os.environ.get("CODESNIPS_DEBUG"):
                import traceback

                traceback.print_exc()
        finally:
            # Avoid writing terminal control sequences while an unsafe foreground
            # app is active; better to leak our scroll region than to corrupt UI.
            safe_to_cleanup = not dock_active or not self._should_pause_dock()
            if safe_to_cleanup:
                try:
                    terminal_dock.cleanup()
                except (BrokenPipeError, OSError):
                    terminal_dock.best_effort_reset()
            self._restore_signal_handlers()
            self._unregister_pid_file()

    def list_terms(self):
        terms = sorted(self.snippets.keys())
        if not terms:
            self.console.print("\n[yellow]No snippets loaded.[/yellow]\n")
            return

        self.console.print("\n[bold]Available terms:[/bold]\n")
        columns = 4
        col_width = max(len(t) for t in terms) + 2

        for i in range(0, len(terms), columns):
            row = terms[i : i + columns]
            line = "  ".join(f"• {t:<{col_width - 2}}" for t in row)
            self.console.print(f"  {line}")

        self.console.print(f"\n[dim]Total: {len(terms)} terms[/dim]\n")

    def search(self, query: str):
        query = query.lower()
        matches = []

        for key, data in self.snippets.items():
            if (
                query in key.lower()
                or query in data.get("term", "").lower()
                or query in data.get("definition", "").lower()
            ):
                matches.append((key, data))

        if not matches:
            self.console.print(f"[yellow]No matches found for: {query}[/yellow]")
            return

        self.console.print(f"\n[bold]Found {len(matches)} match(es):[/bold]\n")

        for key, snippet in matches[:5]:
            term = snippet.get("term", key)
            definition = snippet.get("definition", "No definition available")
            self.console.print(f"  [bold cyan]{term}[/bold cyan]")
            self.console.print(
                f"  [dim]{definition[:100]}{'...' if len(definition) > 100 else ''}[/dim]\n"
            )

        if len(matches) > 5:
            self.console.print(f"  [dim]... and {len(matches) - 5} more[/dim]\n")


def _proxy_binary_candidates() -> list[Path]:
    script_dir = Path(__file__).resolve().parent
    candidates = [
        script_dir / "snips-proxy",
        script_dir / "bin" / "snips-proxy",
        Path.home() / ".local" / "bin" / "snips-proxy",
    ]

    path_hit = shutil.which("snips-proxy")
    if path_hit:
        candidates.insert(0, Path(path_hit))

    unique: list[Path] = []
    seen: set[Path] = set()
    for path in candidates:
        resolved = path.expanduser()
        if resolved in seen:
            continue
        seen.add(resolved)
        unique.append(resolved)
    return unique


def _find_proxy_binary() -> Optional[Path]:
    for candidate in _proxy_binary_candidates():
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return candidate
    return None


def _build_proxy_binary() -> Optional[Path]:
    go_binary = shutil.which("go")
    if go_binary is None:
        return None

    script_dir = Path(__file__).resolve().parent
    target = Path.home() / ".local" / "bin" / "snips-proxy"
    target.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        prefix="snips-proxy.",
        dir=target.parent,
        delete=False,
    ) as tmp:
        tmp_path = Path(tmp.name)

    try:
        result = subprocess.run(
            [go_binary, "build", "-o", str(tmp_path), "./cmd/snips-proxy"],
            cwd=script_dir,
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            text=True,
        )
        if result.returncode != 0:
            stderr = result.stderr.strip()
            if stderr:
                sys.stderr.write(f"Failed to build snips-proxy:\n{stderr}\n")
            tmp_path.unlink(missing_ok=True)
            return None

        tmp_path.chmod(0o755)
        tmp_path.replace(target)
    finally:
        tmp_path.unlink(missing_ok=True)

    if target.is_file() and os.access(target, os.X_OK):
        return target
    return None


def _dispatch_proxy_mode(argv: list[str]):
    proxy_binary = _find_proxy_binary()
    if proxy_binary is None:
        proxy_binary = _build_proxy_binary()
    if proxy_binary is None:
        sys.stderr.write(
            "snips proxy mode is not available.\n"
            "Run ./install.sh to build/install snips-proxy, then try again.\n"
        )
        sys.exit(2)

    proxy_args = [str(proxy_binary), "--snippets-file", str(Path(__file__).parent / "snippets.json")]
    proxy_args.extend(argv)
    os.execv(str(proxy_binary), proxy_args)


def main():
    raw_args = sys.argv[1:]
    if raw_args:
        if raw_args[0] == "wrap":
            _dispatch_proxy_mode(raw_args[1:])
        if raw_args[0] == "--proxy":
            _dispatch_proxy_mode(raw_args[1:])

    dock_arg_explicit = "--dock" in sys.argv
    parser = argparse.ArgumentParser(
        description="codesnips - A lightweight terminal learning tool",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  snips                          Show a random snippet
  snips wrap                     Run a proxy shell with a protected top bar
  snips wrap -- codex            Run a specific command inside the proxy
  snips --run                    Keep a fixed snippet dock at the top of the terminal
  snips --run --dock bottom      Keep the dock at the bottom instead
  snips --run -i 60 --height 7   Run with a 60 second interval and taller dock
  snips --run --dock none        Use the old scrolling mode
  snips docker                   Show snippet about docker
  snips --list                   List all available terms
  snips --search api             Search for terms containing 'api'
""",
    )

    parser.add_argument("term", nargs="?", help="Specific term to display")
    parser.add_argument("-r", "--run", action="store_true", help="Run continuously")
    parser.add_argument(
        "-i",
        "--interval",
        type=int,
        default=30,
        help="Interval between snippets (seconds)",
    )
    parser.add_argument(
        "-l", "--list", action="store_true", help="List all available terms"
    )
    parser.add_argument("-s", "--search", type=str, help="Search for a term")
    parser.add_argument("-f", "--file", type=str, help="Custom snippets JSON file")
    parser.add_argument(
        "--no-clear", action="store_true", help="Don't clear screen between snippets"
    )
    parser.add_argument(
        "--dock",
        choices=["top", "bottom", "none"],
        default="top",
        help="Reserve a fixed terminal region while running continuously",
    )
    parser.add_argument(
        "--height",
        type=int,
        default=6,
        help="Height of the docked snippet region in terminal rows",
    )
    parser.add_argument(
        "--stop",
        action="store_true",
        help="Stop a background codesnips runner",
    )
    parser.add_argument(
        "--no-pause",
        action="store_true",
        help="Don't pause dock refresh for foreground apps (may corrupt TUIs)",
    )
    parser.add_argument(
        "--pause-on",
        type=str,
        default="claude",
        help="Comma-separated substrings that pause dock refresh (default: claude)",
    )

    args = parser.parse_args()

    snips = CodeSnips(snippets_file=args.file)

    if args.list:
        snips.list_terms()
    elif args.search:
        snips.search(args.search)
    elif args.stop:
        snips.stop(dock=args.dock if dock_arg_explicit else None)
    elif args.run:
        snips.run(
            interval=args.interval,
            clear=not args.no_clear,
            dock=args.dock,
            height=args.height,
            pause_on=[t.strip() for t in args.pause_on.split(",") if t.strip()],
            pause_enabled=not args.no_pause,
        )
    elif args.term:
        snips.display_snippet(args.term)
    else:
        snips.display_snippet()


if __name__ == "__main__":
    main()
