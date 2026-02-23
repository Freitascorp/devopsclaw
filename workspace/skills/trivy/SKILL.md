```skill
---
name: trivy
description: "Scan containers, filesystems, and IaC for vulnerabilities and misconfigurations with Trivy. CVE scanning, SBOM generation, and CI/CD integration."
metadata: {"nanobot":{"emoji":"🛡️","requires":{"bins":["trivy"]},"install":[{"id":"brew","kind":"brew","formula":"trivy","bins":["trivy"],"label":"Install Trivy (brew)"}]}}
---

# Trivy Security Scanner Skill

Use `trivy` to scan container images, filesystems, Git repos, and IaC configs for vulnerabilities and misconfigurations.

## Container Image Scanning

```bash
# Scan an image
trivy image nginx:latest

# Scan with severity filter
trivy image --severity HIGH,CRITICAL nginx:latest

# Scan and fail on critical (CI/CD gate)
trivy image --exit-code 1 --severity CRITICAL myapp:v2.1.0

# JSON output
trivy image --format json -o results.json nginx:latest

# Table output (default)
trivy image --format table nginx:latest

# Scan with specific vulnerability types
trivy image --vuln-type os,library nginx:latest

# Ignore unfixed vulnerabilities
trivy image --ignore-unfixed nginx:latest

# Scan from a tar archive
docker save myapp:latest | trivy image --input -
```

## Filesystem Scanning

```bash
# Scan current directory (dependencies, secrets)
trivy fs .

# Scan with severity filter
trivy fs --severity HIGH,CRITICAL --exit-code 1 .

# Scan specific path
trivy fs /path/to/project

# Secret scanning
trivy fs --scanners secret .
```

## IaC Scanning

```bash
# Scan Terraform files
trivy config ./terraform/

# Scan Kubernetes manifests
trivy config ./k8s/

# Scan Dockerfile
trivy config --file-patterns "Dockerfile" .

# Scan Helm charts
trivy config ./charts/myapp/

# All IaC misconfigurations with severity
trivy config --severity HIGH,CRITICAL ./
```

## Kubernetes Cluster Scanning

```bash
# Scan running cluster
trivy k8s --report summary cluster

# Scan specific namespace
trivy k8s --namespace my-app --report all

# Scan specific resource
trivy k8s --report all deployment/web
```

## SBOM (Software Bill of Materials)

```bash
# Generate SBOM in CycloneDX format
trivy image --format cyclonedx -o sbom.json nginx:latest

# Generate SBOM in SPDX format
trivy image --format spdx-json -o sbom.spdx.json nginx:latest

# Scan from existing SBOM
trivy sbom sbom.json
```

## Git Repository Scanning

```bash
# Scan a remote repo
trivy repo https://github.com/org/myapp

# Scan for secrets in repo
trivy repo --scanners secret https://github.com/org/myapp
```

## CI/CD Integration

```bash
# CI gate: fail build on HIGH or CRITICAL
trivy image --exit-code 1 --severity HIGH,CRITICAL --no-progress myapp:$CI_COMMIT_SHA

# Generate JUnit report
trivy image --format template --template "@contrib/junit.tpl" -o junit-report.xml myapp:latest

# Generate SARIF (for GitHub Security)
trivy image --format sarif -o trivy-results.sarif myapp:latest
```

## Configuration

```bash
# Ignore specific CVEs (.trivyignore file)
echo "CVE-2023-12345" >> .trivyignore
echo "CVE-2023-67890" >> .trivyignore

# Skip specific directories
trivy fs --skip-dirs node_modules,vendor,.git .

# Update vulnerability database
trivy image --download-db-only
```

## Tips

- Use `--exit-code 1` in CI to break builds on vulnerabilities.
- Use `--ignore-unfixed` to focus on actionable vulnerabilities.
- Use `.trivyignore` to suppress accepted risks.
- Use `--severity CRITICAL` in CI gates; handle HIGH in sprint work.
- Run `trivy image --download-db-only` as a CI cache step to speed up scans.
- Use `--format sarif` for GitHub Code Scanning integration.
- Scan both images AND filesystem — image scan catches OS packages, fs scan catches app dependencies.
```
