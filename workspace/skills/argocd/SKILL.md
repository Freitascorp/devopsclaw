---
name: argocd
description: "Manage GitOps continuous delivery with Argo CD. Applications, sync, rollbacks, projects, repositories, and multi-cluster deployments."
metadata: {"nanobot":{"emoji":"🐙","requires":{"bins":["argocd"]},"install":[{"id":"brew","kind":"brew","formula":"argocd","bins":["argocd"],"label":"Install Argo CD CLI (brew)"}]}}
---

# Argo CD Skill

Use the `argocd` CLI for GitOps-based Kubernetes deployments. Argo CD watches Git repos and auto-syncs Kubernetes manifests.

## Authentication

```bash
# Login
argocd login argocd.example.com --username admin --password PASSWORD

# Login with SSO
argocd login argocd.example.com --sso

# Get initial admin password (fresh install)
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Update password
argocd account update-password

# Context (multiple servers)
argocd context
argocd context argocd.example.com
```

## Applications

```bash
# List applications
argocd app list

# Create an application
argocd app create myapp \
  --repo https://github.com/org/repo.git \
  --path k8s/production \
  --dest-server https://kubernetes.default.svc \
  --dest-namespace my-app \
  --project default \
  --sync-policy automated \
  --auto-prune \
  --self-heal

# Create from Helm chart
argocd app create myapp \
  --repo https://charts.bitnami.com/bitnami \
  --helm-chart nginx \
  --revision 15.3.1 \
  --dest-server https://kubernetes.default.svc \
  --dest-namespace web \
  --values-literal-file values-prod.yaml

# Get application details
argocd app get myapp

# Get application status (sync, health)
argocd app get myapp -o json | jq '{sync:.status.sync.status,health:.status.health.status}'

# Show resource tree
argocd app resources myapp

# Show manifests that would be applied
argocd app manifests myapp

# Diff (what would change)
argocd app diff myapp
```

## Sync (Deploy)

```bash
# Sync an application (deploy)
argocd app sync myapp

# Sync specific resources
argocd app sync myapp --resource apps:Deployment:web

# Sync with prune (delete removed resources)
argocd app sync myapp --prune

# Force sync (ignore differences)
argocd app sync myapp --force

# Sync dry run
argocd app sync myapp --dry-run

# Wait for sync to complete
argocd app wait myapp --health --timeout 300
```

## Rollback

```bash
# View history
argocd app history myapp

# Rollback to a previous revision
argocd app rollback myapp HISTORY_ID

# Rollback to previous
argocd app rollback myapp
```

## Projects

```bash
# List projects
argocd proj list

# Create a project
argocd proj create team-a \
  --description "Team A applications" \
  --src https://github.com/org/* \
  --dest https://kubernetes.default.svc,team-a-*

# Add allowed namespace
argocd proj add-destination team-a https://kubernetes.default.svc team-a-prod

# Add allowed repo
argocd proj add-source team-a https://github.com/org/team-a-infra.git
```

## Repositories

```bash
# List repos
argocd repo list

# Add a repo (HTTPS)
argocd repo add https://github.com/org/repo.git --username git --password TOKEN

# Add a repo (SSH)
argocd repo add git@github.com:org/repo.git --ssh-private-key-path ~/.ssh/argocd

# Add Helm repo
argocd repo add https://charts.bitnami.com/bitnami --type helm --name bitnami
```

## Clusters (Multi-cluster)

```bash
# List clusters
argocd cluster list

# Add a cluster
argocd cluster add my-cluster-context --name production

# Remove a cluster
argocd cluster rm https://cluster-api.example.com
```

## Application YAML (Declarative)

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: myapp
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/org/repo.git
    targetRevision: main
    path: k8s/production
  destination:
    server: https://kubernetes.default.svc
    namespace: my-app
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
```

## ApplicationSet (Multiple Apps)

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: team-apps
  namespace: argocd
spec:
  generators:
    - git:
        repoURL: https://github.com/org/apps.git
        revision: main
        directories:
          - path: apps/*
  template:
    metadata:
      name: '{{path.basename}}'
    spec:
      project: default
      source:
        repoURL: https://github.com/org/apps.git
        targetRevision: main
        path: '{{path}}'
      destination:
        server: https://kubernetes.default.svc
        namespace: '{{path.basename}}'
```

## Troubleshooting

```bash
# Check app sync errors
argocd app get myapp --show-operation

# View app events
argocd app get myapp -o json | jq '.status.conditions'

# Check ArgoCD server logs
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-application-controller --tail=50

# Refresh app (re-read from Git)
argocd app get myapp --refresh

# Hard refresh (clear cache)
argocd app get myapp --hard-refresh
```

## Tips

- Use `--sync-policy automated` for true GitOps — push to Git and ArgoCD deploys automatically.
- Use `--self-heal` to auto-correct manual kubectl changes.
- Use `--auto-prune` to delete resources removed from Git.
- Use ApplicationSets for managing many similar applications.
- Use `argocd app diff` before `argocd app sync` to preview changes.
- Use Argo CD Notifications for Slack/email alerts on sync status.
- Use `syncOptions: [ServerSideApply=true]` for large CRDs.
