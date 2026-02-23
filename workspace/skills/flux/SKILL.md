```skill
---
name: flux
description: "Manage GitOps continuous delivery with Flux CD. Sources, kustomizations, Helm releases, image automation, and multi-tenancy."
metadata: {"nanobot":{"emoji":"🔄","requires":{"bins":["flux"]},"install":[{"id":"brew","kind":"brew","formula":"fluxcd/tap/flux","bins":["flux"],"label":"Install Flux CLI (brew)"}]}}
---

# Flux CD Skill

Use the `flux` CLI for GitOps-based Kubernetes deployments. Flux reconciles Git repos and Helm charts to your cluster continuously.

## Bootstrap

```bash
# Check prerequisites
flux check --pre

# Bootstrap with GitHub
flux bootstrap github \
  --owner=my-org \
  --repository=fleet-infra \
  --branch=main \
  --path=clusters/production \
  --personal

# Bootstrap with GitLab
flux bootstrap gitlab \
  --owner=my-org \
  --repository=fleet-infra \
  --branch=main \
  --path=clusters/production

# Check Flux status
flux check

# Uninstall Flux
flux uninstall
```

## Sources

```bash
# List sources
flux get sources all

# Create a Git source
flux create source git myapp \
  --url=https://github.com/org/myapp \
  --branch=main \
  --interval=1m

# Create a Git source (SSH)
flux create source git myapp \
  --url=ssh://git@github.com/org/myapp \
  --branch=main \
  --secret-ref=myapp-ssh

# Create a Helm repository source
flux create source helm bitnami \
  --url=https://charts.bitnami.com/bitnami \
  --interval=1h

# Create an OCI source (container registry)
flux create source oci myapp \
  --url=oci://ghcr.io/org/myapp-manifests \
  --tag=latest

# Reconcile a source (force pull)
flux reconcile source git myapp
```

## Kustomizations

```bash
# List kustomizations
flux get kustomizations

# Create a kustomization
flux create kustomization myapp \
  --source=GitRepository/myapp \
  --path=./k8s/production \
  --prune=true \
  --interval=5m \
  --target-namespace=my-app

# Create with health checks
flux create kustomization myapp \
  --source=GitRepository/myapp \
  --path=./k8s/production \
  --prune=true \
  --health-check="Deployment/web.my-app" \
  --health-check="Deployment/api.my-app"

# Reconcile (force sync)
flux reconcile kustomization myapp

# Suspend/resume
flux suspend kustomization myapp
flux resume kustomization myapp

# Delete
flux delete kustomization myapp
```

## Helm Releases

```bash
# List Helm releases
flux get helmreleases

# Create a Helm release
flux create helmrelease nginx \
  --source=HelmRepository/bitnami \
  --chart=nginx \
  --chart-version="15.x" \
  --target-namespace=web \
  --create-target-namespace \
  --values=./values-prod.yaml

# Reconcile
flux reconcile helmrelease nginx

# Suspend/resume
flux suspend helmrelease nginx
flux resume helmrelease nginx
```

## Image Automation

```bash
# Create image repository (watch for new tags)
flux create image repository myapp \
  --image=ghcr.io/org/myapp \
  --interval=5m

# Create image policy (semver filter)
flux create image policy myapp \
  --image-ref=myapp \
  --select-semver=">=1.0.0"

# Create image update automation (auto-commit new tags)
flux create image update myapp \
  --git-repo-ref=myapp \
  --git-repo-path=./k8s/production \
  --checkout-branch=main \
  --push-branch=main \
  --author-name=fluxbot \
  --author-email=flux@example.com \
  --commit-template="chore: update myapp to {{.NewTag}}"

# List image policies
flux get image policy
flux get image repository
```

## Multi-Tenancy

```bash
# Create a tenant namespace
flux create tenant team-a \
  --with-namespace=team-a-prod \
  --with-namespace=team-a-staging

# Create service account for tenant
flux create kustomization team-a \
  --source=GitRepository/team-a-infra \
  --path=./manifests \
  --service-account=team-a \
  --target-namespace=team-a-prod
```

## Monitoring & Troubleshooting

```bash
# Get all Flux resources
flux get all

# Show events
flux events

# Show events for specific resource
flux events --for Kustomization/myapp

# Logs
flux logs --all-namespaces --tail=50

# Logs for specific controller
flux logs --kind=Kustomization --name=myapp

# Trace (show full reconciliation chain)
flux trace myapp --kind=deployment --namespace=my-app

# Export current state
flux export source git myapp > source.yaml
flux export kustomization myapp > ks.yaml
```

## Flux YAML Manifests

### GitRepository:
```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: myapp
  namespace: flux-system
spec:
  interval: 1m
  url: https://github.com/org/myapp
  ref:
    branch: main
```

### Kustomization:
```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: myapp
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: myapp
  path: ./k8s/production
  prune: true
  targetNamespace: my-app
  healthChecks:
    - apiVersion: apps/v1
      kind: Deployment
      name: web
      namespace: my-app
```

### HelmRelease:
```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: nginx
  namespace: web
spec:
  interval: 10m
  chart:
    spec:
      chart: nginx
      version: "15.x"
      sourceRef:
        kind: HelmRepository
        name: bitnami
        namespace: flux-system
  values:
    replicaCount: 3
    service:
      type: LoadBalancer
```

## Tips

- Use `flux reconcile` to force immediate sync without waiting for the interval.
- Use `--prune=true` on Kustomizations to auto-delete removed resources.
- Use `flux diff kustomization myapp` to preview changes before they apply.
- Use SOPS or Sealed Secrets for managing encrypted secrets in Git.
- Use `dependsOn` in Kustomizations to control deployment order.
- Use `flux tree kustomization myapp` to see the dependency tree.
```
