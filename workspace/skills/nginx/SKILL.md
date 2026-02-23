---
name: nginx
description: "Configure, manage, and troubleshoot Nginx as a reverse proxy, load balancer, and web server. Config syntax, SSL/TLS, rate limiting, and debugging."
metadata: {"nanobot":{"emoji":"🟩","requires":{"bins":["nginx"]}}}
---

# Nginx Skill

Manage Nginx as a web server, reverse proxy, and load balancer.

## Service Management

```bash
# Test config syntax
nginx -t
sudo nginx -t

# Reload (graceful — no downtime)
sudo nginx -s reload
sudo systemctl reload nginx

# Start / stop / restart
sudo systemctl start nginx
sudo systemctl stop nginx
sudo systemctl restart nginx

# Status
sudo systemctl status nginx

# Show compiled modules
nginx -V 2>&1 | tr -- - '\n' | grep module
```

## Common Configurations

### Reverse proxy:
```nginx
server {
    listen 80;
    server_name app.example.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### SSL/TLS with Let's Encrypt:
```nginx
server {
    listen 443 ssl http2;
    server_name app.example.com;

    ssl_certificate /etc/letsencrypt/live/app.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/app.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 80;
    server_name app.example.com;
    return 301 https://$host$request_uri;
}
```

### Load balancer:
```nginx
upstream backend {
    least_conn;  # or: round-robin (default), ip_hash, hash
    server 10.0.1.10:8080 weight=3;
    server 10.0.1.11:8080;
    server 10.0.1.12:8080;
    server 10.0.1.13:8080 backup;
}

server {
    listen 80;
    server_name api.example.com;

    location / {
        proxy_pass http://backend;
        proxy_set_header Host $host;
        proxy_next_upstream error timeout http_502 http_503;
    }
}
```

### WebSocket proxy:
```nginx
location /ws {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 86400;
}
```

### Rate limiting:
```nginx
http {
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;

    server {
        location /api/ {
            limit_req zone=api burst=20 nodelay;
            proxy_pass http://127.0.0.1:3000;
        }
    }
}
```

### Static files + caching:
```nginx
server {
    listen 80;
    server_name static.example.com;
    root /var/www/static;

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff2)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }

    location / {
        try_files $uri $uri/ /index.html;
    }

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml;
    gzip_min_length 1000;
}
```

### Security headers:
```nginx
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Content-Security-Policy "default-src 'self'" always;
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

## Troubleshooting

```bash
# Test config
sudo nginx -t

# Check error log
sudo tail -50 /var/log/nginx/error.log

# Check access log
sudo tail -50 /var/log/nginx/access.log

# Real-time access log
sudo tail -f /var/log/nginx/access.log

# Check which process is using port 80
sudo lsof -i :80

# Check open connections
ss -tlnp | grep nginx

# Debug upstream issues
# Add to server block:
# error_log /var/log/nginx/debug.log debug;
```

## File Locations

| File | Purpose |
|---|---|
| `/etc/nginx/nginx.conf` | Main config |
| `/etc/nginx/sites-available/` | Available virtual hosts |
| `/etc/nginx/sites-enabled/` | Enabled virtual hosts (symlinks) |
| `/etc/nginx/conf.d/` | Additional config snippets |
| `/var/log/nginx/access.log` | Access log |
| `/var/log/nginx/error.log` | Error log |

```bash
# Enable a site
sudo ln -s /etc/nginx/sites-available/myapp /etc/nginx/sites-enabled/
sudo nginx -t && sudo nginx -s reload

# Disable a site
sudo rm /etc/nginx/sites-enabled/myapp
sudo nginx -s reload
```

## Tips

- Always run `nginx -t` before reloading — a bad config will break the reload.
- Use `nginx -s reload` (not restart) for zero-downtime config changes.
- Use `proxy_next_upstream` for automatic failover to healthy backends.
- Use `include` to keep configs modular: `include /etc/nginx/conf.d/*.conf;`.
- Use `$request_id` for request tracing across proxy hops.
