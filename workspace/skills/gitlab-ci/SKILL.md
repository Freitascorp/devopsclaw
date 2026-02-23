```skill
---
name: gitlab-ci
description: "Manage GitLab CI/CD pipelines using the `glab` CLI. Pipelines, jobs, merge requests, environments, and deployment management."
metadata: {"nanobot":{"emoji":"🦊","requires":{"bins":["glab"]},"install":[{"id":"brew","kind":"brew","formula":"glab","bins":["glab"],"label":"Install GitLab CLI (brew)"}]}}
---

# GitLab CI/CD Skill

Use the `glab` CLI to interact with GitLab pipelines, jobs, and CI/CD features.

## Authentication

```bash
# Login
glab auth login

# Check status
glab auth status

# Set default project
glab repo set-default owner/repo
```

## Pipelines

```bash
# List recent pipelines
glab ci list

# View pipeline status
glab ci view PIPELINE_ID

# Trigger a pipeline
glab ci run --branch main

# Trigger with variables
glab ci run --branch main --variables "DEPLOY_ENV=production,VERSION=v2.1.0"

# Cancel a pipeline
glab ci cancel PIPELINE_ID

# Retry a pipeline
glab ci retry PIPELINE_ID

# Delete a pipeline
glab ci delete PIPELINE_ID

# Get pipeline status for current branch
glab ci status
```

## Jobs

```bash
# List jobs in a pipeline
glab ci job list --pipeline PIPELINE_ID

# View job logs
glab ci trace JOB_ID

# Retry a specific job
glab ci job retry JOB_ID

# Play a manual job
glab ci job play JOB_ID

# Download job artifacts
glab ci artifact download JOB_ID
```

## Merge Requests

```bash
# List MRs
glab mr list

# Create MR
glab mr create --title "feat: add monitoring" --target-branch main

# View MR pipeline status
glab mr view MR_NUMBER

# Merge
glab mr merge MR_NUMBER

# Approve
glab mr approve MR_NUMBER
```

## Common .gitlab-ci.yml Patterns

### Basic pipeline:
```yaml
stages: [test, build, deploy]

test:
  stage: test
  image: node:20
  script:
    - npm ci
    - npm test

build:
  stage: build
  image: docker:24
  services: [docker:24-dind]
  script:
    - docker build -t $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA .
    - docker push $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA

deploy:
  stage: deploy
  script:
    - ./deploy.sh $CI_COMMIT_SHA
  environment:
    name: production
    url: https://app.example.com
  when: manual
  only: [main]
```

### Multi-environment:
```yaml
.deploy_template: &deploy
  stage: deploy
  script: ./deploy.sh $CI_ENVIRONMENT_NAME $VERSION

deploy_staging:
  <<: *deploy
  environment: { name: staging }
  only: [main]

deploy_production:
  <<: *deploy
  environment: { name: production }
  when: manual
  only: [tags]
```

## Variables & Secrets

```bash
# Variables are managed via GitLab UI or API:
glab api projects/:id/variables
glab api projects/:id/variables --method POST \
  -f key=AWS_ACCESS_KEY -f value=AKIA... -f masked=true -f protected=true
```

## Tips

- Use `glab ci status --live` to watch pipeline progress in real-time.
- Use `--json` for machine-readable output.
- Use `glab ci lint` to validate `.gitlab-ci.yml` syntax.
- Use `include:` in `.gitlab-ci.yml` to share pipeline templates across repos.
```
