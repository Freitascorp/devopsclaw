```skill
---
name: cloudflare
description: "Manage Cloudflare DNS, WAF, caching, tunnels, and Workers using the `flarectl` CLI or Cloudflare API. DNS records, page rules, and Zero Trust tunnels."
metadata: {"nanobot":{"emoji":"🟧","requires":{"bins":["curl"]}}}
---

# Cloudflare Skill

Manage Cloudflare via its REST API using `curl`. Also supports `flarectl` CLI if installed.

## Setup

```bash
export CF_API_TOKEN="your-api-token"  # recommended (scoped)
# Or legacy:
export CF_API_EMAIL="you@example.com"
export CF_API_KEY="your-global-api-key"

export CF_ZONE_ID="zone-id-here"
export CF_API="https://api.cloudflare.com/client/v4"
```

## DNS Records

```bash
# List DNS records
curl -s -H "Authorization: Bearer $CF_API_TOKEN" \
  "$CF_API/zones/$CF_ZONE_ID/dns_records" | jq '.result[] | {id,type,name,content,proxied,ttl}'

# Create a DNS record
curl -s -X POST -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/dns_records" -d '{
    "type": "A",
    "name": "app.example.com",
    "content": "10.0.1.10",
    "ttl": 1,
    "proxied": true
  }'

# Update a DNS record
curl -s -X PUT -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/dns_records/RECORD_ID" -d '{
    "type": "A",
    "name": "app.example.com",
    "content": "10.0.1.11",
    "ttl": 1,
    "proxied": true
  }'

# Delete a DNS record
curl -s -X DELETE -H "Authorization: Bearer $CF_API_TOKEN" \
  "$CF_API/zones/$CF_ZONE_ID/dns_records/RECORD_ID"

# Find record ID by name
curl -s -H "Authorization: Bearer $CF_API_TOKEN" \
  "$CF_API/zones/$CF_ZONE_ID/dns_records?name=app.example.com" | jq '.result[0].id'
```

## Cache

```bash
# Purge everything
curl -s -X POST -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/purge_cache" -d '{"purge_everything": true}'

# Purge specific URLs
curl -s -X POST -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/purge_cache" -d '{
    "files": ["https://example.com/styles.css", "https://example.com/app.js"]
  }'

# Purge by tag
curl -s -X POST -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/purge_cache" -d '{"tags": ["static-assets"]}'
```

## Cloudflare Tunnel (cloudflared)

```bash
# Install cloudflared
brew install cloudflared  # or download from cloudflare

# Login
cloudflared tunnel login

# Create a tunnel
cloudflared tunnel create my-tunnel

# List tunnels
cloudflared tunnel list

# Run a tunnel
cloudflared tunnel run my-tunnel

# Quick tunnel (no config needed)
cloudflared tunnel --url http://localhost:3000
```

### Tunnel config (~/.cloudflared/config.yml):
```yaml
tunnel: TUNNEL_UUID
credentials-file: /root/.cloudflared/TUNNEL_UUID.json

ingress:
  - hostname: app.example.com
    service: http://localhost:3000
  - hostname: api.example.com
    service: http://localhost:8080
  - service: http_status:404
```

## Zone Settings

```bash
# Get zone details
curl -s -H "Authorization: Bearer $CF_API_TOKEN" \
  "$CF_API/zones/$CF_ZONE_ID" | jq '{name:.result.name,status:.result.status,plan:.result.plan.name}'

# Enable Always HTTPS
curl -s -X PATCH -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/settings/always_use_https" -d '{"value": "on"}'

# Set SSL mode
curl -s -X PATCH -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/settings/ssl" -d '{"value": "full"}'

# Set min TLS version
curl -s -X PATCH -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  "$CF_API/zones/$CF_ZONE_ID/settings/min_tls_version" -d '{"value": "1.2"}'
```

## Analytics

```bash
# Zone analytics (last 24h)
curl -s -H "Authorization: Bearer $CF_API_TOKEN" \
  "$CF_API/zones/$CF_ZONE_ID/analytics/dashboard?since=-1440&until=0" \
  | jq '.result.totals | {requests:.requests.all,cached:.requests.cached,threats:.threats.all,bandwidth:.bandwidth.all}'
```

## Tips

- Use API tokens (not Global API Key) — they can be scoped to specific zones and permissions.
- Use `proxied: true` for records that should go through Cloudflare (CDN, WAF, DDoS).
- Use `proxied: false` for records that need direct access (MX, SSH).
- Cloudflare Tunnels replace the need for public IPs and VPNs — perfect for NAT'd servers.
- Use `CF_ZONE_ID` from the Cloudflare dashboard Overview page (right sidebar).
```
