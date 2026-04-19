<p align="center">
  <img src="assets/crit_logo.png" alt="crit" width="300">
</p>

# crit

TUI for reviewing AI-generated code and plans — built for human-in-the-loop agentic coding workflows.

Read a plan or review code changes across multiple files, leave inline comments, and let Claude Code address your feedback automatically.

Built for the human-in-the-loop workflow: Your agent the writes code or a plan, you review it in a TUI, your agent seamlessly reads your comments and makes changes.

![crit code review demo](demo/code-review.gif)

## Install

### Claude Code Plugin Marketplace (recommended)

crit is available as a Claude Code plugin. Add the marketplace and install:

```
/plugin marketplace add kevindutra/crit
/plugin install crit
```

Then use `/crit:review` in Claude Code. It will ask whether you want to review code changes or a document, open the TUI, and after you close it, Claude reads your comments and makes changes.

### From source

```bash
go install github.com/kevindutra/crit/cmd/crit@latest
```

Make sure `$GOPATH/bin` (defaults to `~/go/bin`) is in your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Manual skill install

If you prefer not to use the plugin, you can install the skill directly:

```bash
crit setup-claude          # Install globally (~/.claude/skills/)
crit setup-claude --project # Install for current project only
```

Then use `/crit-review <path>` in Claude Code for document reviews, or `/crit-code-review` for multi-file code reviews.

## Requirements

- **Go 1.21+** for building from source
- **tmux** — required for the Claude Code integration. Crit opens the review TUI in a tmux split pane next to Claude Code.

### Starting a tmux session

If you're not already in tmux, start one before launching Claude Code:

```bash
tmux new -s work
# Now launch Claude Code inside this tmux session
claude
```

If you forget, crit will tell you — but the split-pane review won't work outside of tmux.

## Code Review (multi-file)

```bash
crit review --code
```

Detects changed files in your git repo and opens a tabbed TUI with syntax highlighting, diff markers, and inline commenting across all changed files.

- Diffs against unstaged changes by default, falls back to `HEAD~1` or `main`
- Green gutter markers highlight changed lines
- Comments are aggregated across all files in the session

```bash
# Get all code review comments as JSON
crit status --code
```

### How code review works

1. Run `crit review --code` — crit detects changed files and opens the tabbed TUI
2. Navigate between files and leave inline comments on the changes
3. Quit the TUI — comments are saved to `.crit/`
4. `crit status --code` outputs all comments across files as JSON
5. Claude (or any tool) reads the comments and edits the files

## Document Review (single file)

```bash
crit review docs/plans/my-plan.md
```

Opens a full-screen terminal UI with syntax-highlighted markdown, a comment sidebar, and modal overlays for adding/editing comments.

### tmux split pane mode

When running inside tmux, you can open the TUI in a side-by-side split pane:

```bash
# Open review in a tmux split and return immediately
crit review docs/plan.md --detach

# Open review in a tmux split and block until it closes
crit review docs/plan.md --detach --wait
```

This is how the Claude Code skill invokes crit — `--detach --wait` is a single blocking call that opens the TUI next to Claude Code and waits for you to finish reviewing.

### How document review works

1. Claude writes a plan (or you open any markdown file)
2. `crit review <path>` opens the TUI — read through and leave inline comments
3. Comments are stored as JSON in a local `.crit/` directory (gitignored by default)
4. `crit status <path>` outputs comments as JSON for Claude (or any tool) to consume
5. Claude reads the comments, edits the document, and you can re-review

## Keybindings

| Key                                   | Action                                   |
|---------------------------------------|------------------------------------------|
| `j` / `k`                             | Scroll down / up                         |
| `ctrl+d` / `ctrl+u` / `PgDn` / `PgUp` | Half page down / up                      |
| `g` / `G` / `Home` / `End`            | Jump to top / bottom                     |
| `enter`                               | Add comment at current line              |
| `v`                                   | Visual select mode (multi-line comments) |
| `s`                                   | Toggle comment sidebar                   |
| `[` / `]`                             | Jump to prev / next comment              |
| `q`                                   | Save & quit                              |

**Code review only:**

| Key                 | Action                         |
|---------------------|--------------------------------|
| `tab` / `shift+tab` | Next / previous file tab       |
| `n` / `N`           | Jump to next / previous change |
| `/`                 | Search file tabs               |

## Scriptable CLI

```bash
# Add a comment programmatically
crit comment docs/plan.md --line 15 --body "This needs more detail"

# Multi-line comment
crit comment docs/plan.md --line 10 --end-line 20 --body "Rethink this section"

# Get review comments as JSON (single file)
crit status docs/plan.md

# Get all code review comments as JSON
crit status --code
```

## Shell Completions

```bash
# Bash
crit completion bash > /etc/bash_completion.d/crit

# Zsh
crit completion zsh > "${fpath[1]}/_crit"

# Fish
crit completion fish > ~/.config/fish/completions/crit.fish
```

## Development

```bash
go test ./...
go build ./...
go vet ./...
```

## License

MIT
