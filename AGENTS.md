# Agent Instructions

Project rules, architecture and the task tracker (GitHub Issues in the private `duckpuppy/comic-server-tracker` repo) are described in `CLAUDE.md`. Read its "Task Tracker", "Standing Notes" and "Session Completion" sections first. Beads (`bd`) is retired; do not use it for new work.

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

## Task Tracker (GitHub Issues)

Open work is tracked as GitHub Issues in the **private** repo `duckpuppy/comic-server-tracker` (moved from beads on 2026-10-16: cloud threads cannot push beads' Dolt data ref). This repo is public, so keep prod host names, paths and other private detail in the tracker or the handoff notes, not in commits, PR bodies or code. Use `gh` (or the GitHub MCP tools in cloud threads); do NOT use TodoWrite, TaskCreate or markdown TODO lists.

- **Find work**: `gh issue list --repo duckpuppy/comic-server-tracker --label P1` (or `P2`...); `gh issue view N --repo ...`.
- **Priority** is the label `P0` (critical) to `P4` (backlog). **Type** is the label `bug`, `feature`, `task` or `epic`.
- **Create**: `gh issue create --repo duckpuppy/comic-server-tracker --title ... --body ... --label P2,feature`. Put design, acceptance criteria and notes in the body; there is no length pressure.
- **Epics**: make children native sub-issues (cloud MCP: `issue_write` create with `parent_issue_number`, or `sub_issue_write`; CLI: `gh api -X POST repos/duckpuppy/comic-server-tracker/issues/PARENT/sub_issues -F sub_issue_id=<child database id>`).
- **Dependencies**: `gh api -X POST repos/duckpuppy/comic-server-tracker/issues/N/dependencies/blocked_by -F issue_id=<blocker database id>` (get the id with `gh api repos/.../issues/M --jq .id`). No MCP tool exists for this.
- **Close from a PR**: put `Closes duckpuppy/comic-server-tracker#N` in the PR body; merging closes the issue even though the repo differs. The line reveals the private repo name and number in a public PR, nothing else.
- **Old IDs**: the 14 beads still open on 2026-10-16 became issues #1-#14 and keep their `comic-server-xxxx` id in the title. The ~278 closed beads stay in the local beads database (`bd show <id>`) as a read-only archive. Older docs, commits and memory notes cite those ids.
- **Not in the tracker**: durable facts about the code belong in this file; `bd remember` is no longer used.

## Session Completion

See `CLAUDE.md` "Session Completion": quality gates, `git pull --rebase`, `git push`, and `git status` showing up to date with origin. Work is not complete until `git push` succeeds.
