```skill
---
name: cert-manager
description: "Manage TLS certificates in Kubernetes with cert-manager. Issuers, certificates, ACME/Let's Encrypt, and troubleshooting certificate issues."
metadata: {"nanobot":{"emoji":"🔒","requires":{"bins":["kubectl"]}}}
---

# cert-manager Skill

cert-manager runs in Kubernetes and automates TLS certificate management. It works with Let's Encrypt, Vault, Venafi, and self-signed CAs. Manage it via `kubectl`.

## Check Installation

```bash
# Verify cert-manager is running
kubectl get pods -n cert-manager

# Check cert-manager version
kubectl get deployment cert-manager -n cert-manager -o jsonpath='{.spec.template.spec.containers[0].image}'

# Install via Helm (if not installed)
helm install cert-manager jetstack/cert-manager -n cert-manager --create-namespace \
  --set crds.enabled=true
```

## Issuers

### ClusterIssuer (cluster-wide, recommended):
```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@example.com
    privateKeySecretRef:
      name: letsencrypt-prod-key
    solvers:
      - http01:
          ingress:
            class: nginx
```

### Staging issuer (for testing):
```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: admin@example.com
    privateKeySecretRef:
      name: letsencrypt-staging-key
    solvers:
      - http01:
          ingress:
            class: nginx
```

### Self-signed issuer:
```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: self-signed
spec:
  selfSigned: {}
```

```bash
# Apply issuers
kubectl apply -f issuer.yaml

# List issuers
kubectl get clusterissuers
kubectl get issuers -A

# Describe issuer (check Ready condition)
kubectl describe clusterissuer letsencrypt-prod
```

## Certificates

### Request a certificate:
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: app-tls
  namespace: my-app
spec:
  secretName: app-tls-secret
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - app.example.com
    - www.app.example.com
```

### Wildcard certificate (requires DNS solver):
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: wildcard-tls
  namespace: my-app
spec:
  secretName: wildcard-tls-secret
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - "*.example.com"
    - example.com
```

```bash
# Apply certificate
kubectl apply -f certificate.yaml

# List certificates
kubectl get certificates -A
kubectl get cert -A  # short form

# Check certificate status
kubectl describe certificate app-tls -n my-app

# Check the TLS secret
kubectl get secret app-tls-secret -n my-app -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -text | head -20

# Check certificate expiry
kubectl get secret app-tls-secret -n my-app -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -enddate
```

## Ingress Annotations (Automatic)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-ingress
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
    - hosts: [app.example.com]
      secretName: app-tls-auto
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service: { name: app, port: { number: 80 } }
```

cert-manager automatically creates a Certificate resource when it sees the annotation.

## Troubleshooting

```bash
# Check CertificateRequest status
kubectl get certificaterequests -A
kubectl describe certificaterequest APP_TLS_REQUEST -n my-app

# Check Orders (ACME)
kubectl get orders -A
kubectl describe order ORDER_NAME -n my-app

# Check Challenges (ACME)
kubectl get challenges -A
kubectl describe challenge CHALLENGE_NAME -n my-app

# cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager --tail=50

# Common issues:
# - Challenge stuck: check ingress class, DNS propagation, firewall (port 80 open)
# - Rate limited: switch to staging issuer for testing
# - DNS01 challenge: check cloud credentials for DNS provider
# - Certificate not renewing: check cert-manager logs and events
```

## Force Renewal

```bash
# Delete the secret to trigger re-issuance
kubectl delete secret app-tls-secret -n my-app

# Or annotate the certificate to trigger renewal
kubectl annotate certificate app-tls -n my-app cert-manager.io/renew-before="720h" --overwrite

# Or use cmctl
cmctl renew app-tls -n my-app
```

## Tips

- Use `letsencrypt-staging` for testing to avoid hitting rate limits.
- cert-manager auto-renews certificates 30 days before expiry by default.
- Use `cert-manager.io/cluster-issuer` annotation on Ingress for automatic certificate creation.
- For DNS01 challenges (wildcards), configure a DNS provider (Route53, CloudDNS, Cloudflare).
- Install `cmctl` for cert-manager-specific CLI operations.
```
