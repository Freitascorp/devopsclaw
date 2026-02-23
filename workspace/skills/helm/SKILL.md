```skill
---
name: helm
description: "Manage Kubernetes applications with Helm charts. Install, upgrade, rollback releases, manage repositories, and create custom charts."
metadata: {"nanobot":{"emoji":"⎈","requires":{"bins":["helm"]},"install":[{"id":"brew","kind":"brew","formula":"helm","bins":["helm"],"label":"Install Helm (brew)"}]}}
---

# Helm Skill

Use `helm` to package, deploy, and manage Kubernetes applications as charts.

## Repositories

```bash
# Add a repo
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add jetstack https://charts.jetstack.io
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts

# Update repos
helm repo update

# List repos
helm repo list

# Search for a chart
helm search repo nginx
helm search repo bitnami/postgresql --versions
```

## Install & Upgrade

```bash
# Install a chart
helm install my-release bitnami/nginx -n my-namespace

# Install with custom values
helm install my-release bitnami/postgresql -n db \
  --set auth.postgresPassword=secret \
  --set primary.persistence.size=50Gi

# Install with values file
helm install my-release bitnami/nginx -n my-namespace -f values-prod.yaml

# Upgrade a release
helm upgrade my-release bitnami/nginx -n my-namespace -f values-prod.yaml

# Install or upgrade (idempotent)
helm upgrade --install my-release bitnami/nginx -n my-namespace -f values-prod.yaml

# Dry run (preview)
helm install my-release bitnami/nginx --dry-run

# Template (render without installing)
helm template my-release bitnami/nginx -f values-prod.yaml > rendered.yaml
```

## Manage Releases

```bash
# List releases
helm list
helm list -A  # all namespaces
helm list -n my-namespace

# Show release status
helm status my-release -n my-namespace

# Show release values
helm get values my-release -n my-namespace
helm get values my-release -n my-namespace --all  # including defaults

# Show release manifest
helm get manifest my-release -n my-namespace

# Release history
helm history my-release -n my-namespace

# Rollback
helm rollback my-release 1 -n my-namespace  # rollback to revision 1

# Uninstall
helm uninstall my-release -n my-namespace
```

## Chart Development

```bash
# Create a new chart
helm create my-chart

# Chart structure:
# my-chart/
#   Chart.yaml          # metadata
#   values.yaml         # default values
#   templates/          # Kubernetes manifests
#     deployment.yaml
#     service.yaml
#     ingress.yaml
#     _helpers.tpl      # template helpers
#   charts/             # dependencies

# Lint a chart
helm lint my-chart/

# Package a chart
helm package my-chart/

# Show chart info
helm show chart bitnami/nginx
helm show values bitnami/nginx  # default values (useful for customization)
```

## Dependencies

```bash
# List chart dependencies
helm dependency list my-chart/

# Update dependencies
helm dependency update my-chart/

# Build dependencies
helm dependency build my-chart/
```

## Common Helm Charts for DevOps

```bash
# NGINX Ingress Controller
helm install ingress ingress-nginx/ingress-nginx -n ingress-nginx --create-namespace

# cert-manager
helm install cert-manager jetstack/cert-manager -n cert-manager --create-namespace \
  --set crds.enabled=true

# Prometheus + Grafana stack
helm install monitoring prometheus-community/kube-prometheus-stack -n monitoring --create-namespace

# PostgreSQL
helm install db bitnami/postgresql -n database --create-namespace \
  --set auth.postgresPassword=secret

# Redis
helm install cache bitnami/redis -n cache --create-namespace

# ArgoCD
helm install argocd argo/argo-cd -n argocd --create-namespace
```

## Tips

- Use `helm show values CHART` to discover all configurable options before installing.
- Use `--create-namespace` to auto-create the namespace on install.
- Use `--wait` to block until all resources are ready.
- Use `--timeout 10m` for slow deployments.
- Use `helm diff` plugin to see changes before upgrading: `helm diff upgrade my-release bitnami/nginx -f values.yaml`.
- Pin chart versions in CI: `helm install --version 15.3.1`.
- Use `helm secrets` plugin for encrypted values files.
```
