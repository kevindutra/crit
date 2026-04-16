---
name: crit-code-review
description: Review code changes in crit's multi-file TUI with syntax highlighting and diff markers. After the review, address any comments.
allowed-tools: Bash(crit *), Read, Edit, Grep, MultiEdit
---

# Code Review

Review code changes using crit's multi-file code review TUI.

## Prerequisites

The `crit` binary must be installed and on PATH. If not installed:

```bash
go install github.com/kevindutra/crit/cmd/crit@latest
```

## Step 1: Launch the TUI

Check if `$TMUX` is set:

If in tmux, run this command with a **timeout of 600000** (10 minutes) since it blocks until the user finishes reviewing:
```bash
crit review --code --detach --wait
```

If not in tmux (command fails with "requires a tmux session"), ask the user to run the TUI manually:

> Please run this in your terminal, review the changes, and let me know when you're done:
>
> ```
> crit review --code
> ```

Wait for the user to confirm before proceeding.

## Step 2: Read the comments

After the user confirms the review is complete, read the aggregate review comments:

```bash
crit status --code
```

This outputs JSON with all files and their comments.

## Step 3: Address comments

For each file in the `files` array, for each comment:

1. Read the `line` number and `content_snippet` to locate where the comment applies
2. Read the `body` for what the reviewer wants changed
3. Edit the file to address the comment

After addressing ALL comments across ALL files, summarize what you changed.

## Step 4: Prompt for next action

After addressing all comments and summarizing the changes, present the user with exactly these three options:

> I've addressed all your comments. What would you like to do next?
>
> **1. Re-review** — open crit again to review the changes
> **2. Continue** — move on
> **3.** Type anything to give me additional instructions or context

If the user chooses **1**, ask:

> Keep existing comments or clear them before re-reviewing?

If clear, run:
```bash
crit clear --code
```
Then go back to Step 1. If keep, go back to Step 1 directly.

If the user chooses **2**, done.
If the user types anything else, treat it as free-form input and respond accordingly — then present the three options again until the user picks 1 or 2.

## Important notes

- Do NOT modify files while the TUI is open — only edit after it exits
- The `content_snippet` field shows the line content when the comment was created — use it to find the right location even if line numbers have shifted
