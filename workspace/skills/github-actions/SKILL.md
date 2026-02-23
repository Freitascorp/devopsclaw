---
name: github-actions
description: "Manage GitHub Actions CI/CD workflows using the `gh` CLI. List, view, trigger, and debug workflow runs, manage secrets, and view job logs."
metadata: {"nanobot":{"emoji":"⚡","requires":{"bins":["gh"]},"install":[{"id":"brew","kind":"brew","formula":"gh","bins":["gh"],"label":"Install GitHub CLI (brew)"}]}}
---

# GitHub Actions Skill

Use the `gh` CLI to manage GitHub Actions workflows, runs, and secrets. For creating/editing workflow YAML files, use the filesystem tools.

## Workflow Runs

```bash
# List recent runs
gh run list --repo owner/repo --limit 10

# List runs for a specific workflow
gh run list --repo owner/repo --workflow deploy.yml

# List failed runs
gh run list --repo owner/repo --status failure

# View a specific run
gh run view RUN_ID --repo owner/repo

# View run with job details
gh run view RUN_ID --repo owner/repo --verbose

# View failed logs
gh run view RUN_ID --repo owner/repo --log-failed

# Download full logs
gh run view RUN_ID --repo owner/repo --log > run.log

# Watch a run in real-time
gh run watch RUN_ID --repo owner/repo

# Rerun failed jobs
gh run rerun RUN_ID --repo owner/repo --failed

# Rerun entire workflow
gh run rerun RUN_ID --repo owner/repo

# Cancel a run
gh run cancel RUN_ID --repo owner/repo
```

## Trigger Workflows

```bash
# Trigger a workflow_dispatch workflow
gh workflow run deploy.yml --repo owner/repo

# Trigger with inputs
gh workflow run deploy.yml --repo owner/repo \
  -f environment=production -f version=v2.1.0

# Trigger on a specific branch
gh workflow run deploy.yml --repo owner/repo --ref feature-branch

# List workflows
gh workflow list --repo owner/repo

# View workflow definition
gh workflow view deploy.yml --repo owner/repo

# Enable/disable a workflow
gh workflow enable deploy.yml --repo owner/repo
gh workflow disable deploy.yml --repo owner/repo
```

## Secrets & Variables

```bash
# List secrets
gh secret list --repo owner/repo

# Set a secret
gh secret set AWS_ACCESS_KEY_ID --repo owner/repo --body "AKIAIOSFODNN7EXAMPLE"

# Set from file
gh secret set SSH_KEY --repo owner/repo < ~/.ssh/deploy_key

# Set environment secret
gh secret set DB_PASSWORD --repo owner/repo --env production --body "s3cret"

# Delete a secret
gh secret delete OLD_SECRET --repo owner/repo

# List variables
gh variable list --repo owner/repo

# Set a variable
gh variable set DEPLOY_REGION --repo owner/repo --body "us-east-1"
```

## Artifacts & Caches

```bash
# List artifacts from a run
gh run view RUN_ID --repo owner/repo --json artifacts

# Download artifacts
gh run download RUN_ID --repo owner/repo

# Download specific artifact
gh run download RUN_ID --repo owner/repo --name build-output

# List caches
gh cache list --repo owner/repo

# Delete a cache
gh cache delete CACHE_KEY --repo owner/repo
```

## Common Workflow Patterns

### CI pipeline:
```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 20 }
      - run: npm ci
      - run: npm test
      - run: npm run build
```

### Deploy on tag:
```yaml
name: Deploy
on:
  push:
    tags: ['v*']
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4
      - run: ./deploy.sh ${{ github.ref_name }}
```

### Manual dispatch:
```yaml
name: Deploy
on:
  workflow_dispatch:
    inputs:
      environment:
        type: choice
        options: [staging, production]
      version:
        required: true
```

## Debugging Failed Runs

1. List recent failures: `gh run list --status failure --limit 5`
2. View the failed run: `gh run view RUN_ID`
3. Get failed step logs: `gh run view RUN_ID --log-failed`
4. If logs are truncated, download full logs: `gh run view RUN_ID --log > full.log`
5. Rerun just the failed jobs: `gh run rerun RUN_ID --failed`

## Tips

- Use `--json` for programmatic access: `gh run list --json conclusion,name,startedAt`.
- Use `gh act` (nektos/act) to test workflows locally.
- Use `gh api` for advanced operations not covered by `gh run`/`gh workflow`.
- Set `GH_REPO=owner/repo` to avoid repeating `--repo`.
