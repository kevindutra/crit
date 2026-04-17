---
name: crit:plan-review
description: Open a document or plan in crit's interactive TUI for review. After the review, address any comments by editing the document.
argument-hint: <file-path>
---

# Review Document

Review the document at `$ARGUMENTS` using crit's interactive TUI.

## Prerequisites

The `crit` binary must be installed and on PATH. If not installed:

```bash
go install github.com/kevindutra/crit/cmd/crit@latest
```

## Step 1: Launch the TUI

Check if `$TMUX` is set:

If in tmux, run this command with a **timeout of 600000** (10 minutes) since it blocks until the user finishes reviewing:
```bash
crit review $ARGUMENTS --detach --wait
```

If not in tmux (command fails with "requires a tmux session"), ask the user to run the TUI manually:

> Please run this in your terminal, review the document, and let me know when you're done:
>
> ```
> crit review $ARGUMENTS
> ```

Wait for the user to confirm before proceeding.

## Step 2: Read the comments

After the user confirms the review is complete, read the review comments:

```bash
crit status $ARGUMENTS
```

This outputs JSON with the file path and comments array.

## Step 3: Address comments

For each comment in the `comments` array:

1. Read the `line` number and `content_snippet` to locate where in the document the comment applies
2. Read the `body` for what the reviewer wants changed
3. Edit the document at `$ARGUMENTS` to address the comment

After addressing ALL comments, summarize what you changed.

## Step 4: Prompt for next action

After addressing all comments and summarizing the changes, use the `AskUserQuestion` tool to ask:

- **Question:** "I've addressed all your comments. What would you like to do next?"
- **Header:** "Next action"
- **Options:**
  - **Re-review** — Open crit again to review the document
  - **Continue** — Done, move on

If the user provides free-form input (via the "Other" option), respond accordingly, then ask again with `AskUserQuestion` until they pick Re-review or Continue.

If the user chooses **Re-review**, use `AskUserQuestion` again to ask:

- **Question:** "Keep existing comments or clear them before re-reviewing?"
- **Header:** "Comments"
- **Options:**
  - **Keep** — Keep existing comments visible during re-review
  - **Clear** — Remove all comments before re-reviewing

If clear, run:
```bash
crit clear $ARGUMENTS
```
Then go back to Step 1. If keep, go back to Step 1 directly.

If the user chooses **Continue**, done.

## Important notes

- Do NOT modify the document while the TUI is open — only edit after it exits
- The `content_snippet` field shows the line content when the comment was created — use it to find the right location even if line numbers have shifted
