---
name: crit-plan-review
description: Open a document or plan in crit's interactive TUI for review. After the review, address any comments by editing the document. Use when a plan or document needs human review, when the user asks to review a document, or after generating/updating a plan.
allowed-tools: Bash(crit *), Read, Edit, Grep
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

After addressing all comments and summarizing the changes, present the user with exactly these three options:

> I've addressed all your comments. What would you like to do next?
>
> **1. Re-review** — open crit again to review the document
> **2. Continue** — move on
> **3.** Type anything to give me additional instructions or context

If the user chooses **1**, ask:

> Keep existing comments or clear them before re-reviewing?

If clear, run:
```bash
crit clear $ARGUMENTS
```
Then go back to Step 1. If keep, go back to Step 1 directly.

If the user chooses **2**, done.
If the user types anything else, treat it as free-form input and respond accordingly — then present the three options again until the user picks 1 or 2.

## Important notes

- Do NOT modify the document while the TUI is open — only edit after it exits
- The `content_snippet` field shows the line content when the comment was created — use it to find the right location even if line numbers have shifted
