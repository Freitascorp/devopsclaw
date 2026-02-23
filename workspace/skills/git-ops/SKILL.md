---
name: git-ops
description: "Advanced Git operations for DevOps workflows. Branching strategies, rebasing, cherry-picking, bisect, submodules, hooks, and repository maintenance."
metadata: {"nanobot":{"emoji":"📝","requires":{"bins":["git"]}}}
---

# Git for DevOps Skill

Advanced Git operations commonly needed in DevOps workflows — beyond basic add/commit/push.

## Branching & Merging

```bash
# Create and switch
git checkout -b feature/my-feature
git switch -c feature/my-feature  # modern

# List branches (local and remote)
git branch -a
git branch -vv  # with tracking info

# Delete local branch
git branch -d feature/old
git branch -D feature/old  # force

# Delete remote branch
git push origin --delete feature/old

# Merge with no fast-forward (keeps merge commit)
git merge --no-ff feature/my-feature

# Abort a merge
git merge --abort
```

## Rebasing

```bash
# Rebase onto main
git rebase main

# Interactive rebase (squash, reorder, edit)
git rebase -i HEAD~5
git rebase -i main

# Continue after resolving conflicts
git rebase --continue

# Abort rebase
git rebase --abort
```

## Cherry-Pick

```bash
# Apply a specific commit
git cherry-pick abc123

# Cherry-pick without committing (stage only)
git cherry-pick --no-commit abc123

# Cherry-pick a range
git cherry-pick abc123..def456
```

## Tags (Releases)

```bash
# List tags
git tag -l
git tag -l "v2.*"

# Create annotated tag
git tag -a v2.1.0 -m "Release 2.1.0"

# Push tags
git push origin v2.1.0
git push --tags

# Delete a tag
git tag -d v2.0.0-beta
git push origin --delete v2.0.0-beta
```

## Bisect (Find Breaking Commit)

```bash
# Start bisect
git bisect start

# Mark current as bad
git bisect bad

# Mark known-good commit
git bisect good v2.0.0

# Git checks out middle commit — test it, then:
git bisect good  # or
git bisect bad

# Repeat until found, then:
git bisect reset

# Automated bisect
git bisect start HEAD v2.0.0
git bisect run ./test.sh
```

## Stash

```bash
# Stash current changes
git stash
git stash push -m "WIP: feature X"

# List stashes
git stash list

# Apply and remove
git stash pop

# Apply without removing
git stash apply stash@{0}

# Drop a stash
git stash drop stash@{0}
```

## Log & History

```bash
# Compact log
git log --oneline -20

# Graph
git log --oneline --graph --all -20

# Commits by author
git log --author="name" --oneline -10

# Commits touching a file
git log --follow -p -- path/to/file

# Search commit messages
git log --grep="fix" --oneline -10

# Diff between branches
git diff main..feature/x -- path/
git diff main..feature/x --stat

# Show who changed each line
git blame path/to/file

# Commits between dates
git log --since="2025-01-01" --until="2025-02-01" --oneline
```

## Submodules

```bash
# Add submodule
git submodule add https://github.com/org/lib.git vendor/lib

# Initialize and update submodules
git submodule init
git submodule update --init --recursive

# Update submodules to latest
git submodule update --remote

# Remove a submodule
git submodule deinit vendor/lib
git rm vendor/lib
```

## Worktrees (Multiple Checkouts)

```bash
# Create a worktree
git worktree add ../hotfix hotfix-branch

# List worktrees
git worktree list

# Remove a worktree
git worktree remove ../hotfix
```

## Hooks (Automation)

```bash
# Pre-commit hook (.git/hooks/pre-commit)
#!/bin/sh
make lint || exit 1
make test || exit 1

# Pre-push hook
#!/bin/sh
make test || exit 1

# Make hooks executable
chmod +x .git/hooks/pre-commit
```

## Repository Maintenance

```bash
# Garbage collection
git gc --aggressive --prune=now

# Check repo integrity
git fsck

# Find large objects
git rev-list --objects --all | git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' | sort -rnk3 | head -10

# Remove file from all history (BFG Cleaner)
bfg --delete-files "*.env" --no-blob-protection
git reflog expire --expire=now --all && git gc --prune=now
```

## Useful Aliases

```bash
git config --global alias.co checkout
git config --global alias.br branch
git config --global alias.st status
git config --global alias.lg "log --oneline --graph --all -20"
git config --global alias.last "log -1 HEAD --stat"
```

## Tips

- Use `git reflog` to recover lost commits (even after hard reset).
- Use `git stash` before switching branches with uncommitted changes.
- Use `git log --all --oneline | wc -l` to count total commits.
- Use `git shortlog -sn` to see commit counts by author.
- Use `git clean -fd` to remove untracked files and directories.
- Use `--no-pager` or `| cat` to avoid pager in scripts.
