# User

## Profile

- **Name**: (set your name)
- **Role**: DevOps Engineer / SRE / Platform Engineer / Developer
- **Timezone**: (your timezone, e.g., Asia/Bangkok, America/New_York)
- **Language**: English

## Preferences

- **Communication**: Direct and concise — show results, not essays
- **Execution style**: Bias toward action — execute tasks, don't just explain them
- **Confirmations**: Only ask before destructive operations (deploy, delete, modify infra)
- **Skill usage**: Always load and use skills — combine multiple skills for complex tasks
- **Error handling**: Diagnose and retry before asking for help

## Infrastructure Context

*Fill this in so the agent remembers your environment across sessions.*

- **Primary cloud**: (aws / gcp / azure / on-prem / hybrid)
- **Container runtime**: (docker / containerd / podman)
- **Orchestration**: (kubernetes / docker-compose / ecs / nomad)
- **CI/CD**: (github-actions / gitlab-ci / jenkins / argocd / flux)
- **IaC tool**: (terraform / pulumi / cloudformation / ansible)
- **Monitoring**: (prometheus+grafana / datadog / elastic-stack / cloudwatch)
- **Secrets management**: (vault / aws-secrets-manager / azure-keyvault / sealed-secrets)
- **DNS/CDN**: (cloudflare / route53 / cloudfront)

## Common Tasks

*List your frequent tasks so the agent can optimize for them.*

- (e.g., "deploy webapp to staging" → docker + k8s + helm)
- (e.g., "check production health" → kubernetes + prometheus + grafana)
- (e.g., "rotate TLS certs" → cert-manager + vault + nginx)
- (e.g., "run security scan" → trivy + vault)

## Fleet Nodes

*If you use fleet management, describe your node topology.*

- (e.g., "web-01..web-04: nginx + app, tag: role=web")
- (e.g., "db-01, db-02: postgres primary/replica, tag: role=db")
- (e.g., "monitor-01: prometheus + grafana, tag: role=monitoring")

## Notes

*Anything else the agent should know about your setup.*