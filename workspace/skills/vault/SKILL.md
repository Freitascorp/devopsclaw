```skill
---
name: vault
description: "Manage secrets, encryption, and PKI with HashiCorp Vault. Secret engines, auth methods, policies, dynamic credentials, and transit encryption."
metadata: {"nanobot":{"emoji":"🔐","requires":{"bins":["vault"]},"install":[{"id":"brew","kind":"brew","formula":"vault","bins":["vault"],"label":"Install Vault (brew)"}]}}
---

# HashiCorp Vault Skill

Use the `vault` CLI to manage secrets, dynamic credentials, encryption, and PKI certificates.

## Authentication

```bash
# Set Vault address
export VAULT_ADDR="https://vault.example.com:8200"

# Login with token
vault login hvs.XXXXX

# Login with LDAP
vault login -method=ldap username=admin

# Login with AppRole (CI/CD)
vault write auth/approle/login role_id="$ROLE_ID" secret_id="$SECRET_ID"

# Login with AWS IAM
vault login -method=aws role=my-role

# Check auth status
vault token lookup

# Renew token
vault token renew
```

## KV Secrets (v2)

```bash
# Write a secret
vault kv put secret/myapp/db username=admin password=s3cret host=db.example.com

# Read a secret
vault kv get secret/myapp/db

# Read specific field
vault kv get -field=password secret/myapp/db

# Read as JSON
vault kv get -format=json secret/myapp/db | jq '.data.data'

# List secrets
vault kv list secret/myapp/

# Delete a secret (soft delete)
vault kv delete secret/myapp/db

# Undelete
vault kv undelete -versions=2 secret/myapp/db

# View secret history
vault kv metadata get secret/myapp/db

# Permanently destroy a version
vault kv destroy -versions=1 secret/myapp/db

# Patch a secret (update specific fields)
vault kv patch secret/myapp/db password=new-s3cret
```

## Dynamic Database Credentials

```bash
# Configure database secret engine
vault secrets enable database

vault write database/config/mydb \
  plugin_name=postgresql-database-plugin \
  connection_url="postgresql://{{username}}:{{password}}@db.example.com:5432/mydb" \
  allowed_roles="readonly,readwrite" \
  username="vault_admin" \
  password="admin_password"

# Create a role
vault write database/roles/readonly \
  db_name=mydb \
  creation_statements="CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"{{name}}\";" \
  default_ttl="1h" \
  max_ttl="24h"

# Get dynamic credentials
vault read database/creds/readonly
# Returns: username=v-token-readonly-xxxx, password=A1B2C3D4...

# Revoke a lease
vault lease revoke database/creds/readonly/LEASE_ID
```

## AWS Dynamic Credentials

```bash
# Configure AWS secret engine
vault secrets enable aws

vault write aws/config/root \
  access_key=AKIAIOSFODNN7EXAMPLE \
  secret_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY \
  region=us-east-1

# Create a role
vault write aws/roles/deploy \
  credential_type=iam_user \
  policy_arns=arn:aws:iam::aws:policy/AmazonS3FullAccess

# Get temporary AWS credentials
vault read aws/creds/deploy
# Returns: access_key, secret_key, security_token
```

## Transit (Encryption as a Service)

```bash
# Enable transit
vault secrets enable transit

# Create an encryption key
vault write -f transit/keys/my-key

# Encrypt data
vault write transit/encrypt/my-key plaintext=$(echo "sensitive data" | base64)
# Returns: ciphertext=vault:v1:XXXXX

# Decrypt data
vault write transit/decrypt/my-key ciphertext="vault:v1:XXXXX"
# Returns: plaintext (base64 encoded)

# Rotate encryption key
vault write -f transit/keys/my-key/rotate
```

## PKI (Certificate Authority)

```bash
# Enable PKI
vault secrets enable pki
vault secrets tune -max-lease-ttl=87600h pki

# Generate root CA
vault write pki/root/generate/internal \
  common_name="My CA" \
  ttl=87600h

# Create a role for issuing certs
vault write pki/roles/web-server \
  allowed_domains="example.com" \
  allow_subdomains=true \
  max_ttl="720h"

# Issue a certificate
vault write pki/issue/web-server \
  common_name="app.example.com" \
  ttl="72h"
```

## Policies

```bash
# List policies
vault policy list

# Read a policy
vault policy read default

# Write a policy
vault policy write app-read - <<EOF
path "secret/data/myapp/*" {
  capabilities = ["read", "list"]
}
path "database/creds/readonly" {
  capabilities = ["read"]
}
EOF

# Delete a policy
vault policy delete app-read
```

## Audit & Operations

```bash
# Enable audit log
vault audit enable file file_path=/var/log/vault-audit.log

# List secret engines
vault secrets list

# List auth methods
vault auth list

# Seal/unseal
vault operator seal
vault operator unseal UNSEAL_KEY

# Check status
vault status
```

## Tips

- Use `-format=json` for machine-readable output: `vault kv get -format=json secret/myapp/db | jq`.
- Use AppRole for CI/CD — it gives short-lived tokens without storing long-lived secrets.
- Use dynamic credentials (database, AWS) instead of static secrets wherever possible.
- Use `vault kv patch` instead of `vault kv put` to update individual fields without overwriting.
- Set `VAULT_ADDR` and `VAULT_TOKEN` environment variables to avoid repeating them.
- Use `vault lease revoke -prefix database/creds/` to revoke all dynamic creds in an emergency.
```
