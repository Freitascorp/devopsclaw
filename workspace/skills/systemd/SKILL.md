```skill
---
name: systemd
description: "Manage Linux services, timers, and system state with systemd. Create services, debug failures, manage boot targets, and journal logs."
metadata: {"nanobot":{"emoji":"⚙️","requires":{"bins":["systemctl"]}}}
---

# systemd Skill

Manage Linux services, timers, and system state using `systemctl` and `journalctl`.

## Service Management

```bash
# Start / stop / restart / reload
sudo systemctl start myapp
sudo systemctl stop myapp
sudo systemctl restart myapp
sudo systemctl reload myapp       # graceful reload (if supported)
sudo systemctl reload-or-restart myapp

# Enable / disable (boot persistence)
sudo systemctl enable myapp       # start on boot
sudo systemctl disable myapp      # don't start on boot
sudo systemctl enable --now myapp # enable AND start immediately

# Status
systemctl status myapp
systemctl is-active myapp
systemctl is-enabled myapp
systemctl is-failed myapp

# List all services
systemctl list-units --type=service
systemctl list-units --type=service --state=running
systemctl list-units --type=service --state=failed

# List all enabled services
systemctl list-unit-files --type=service --state=enabled
```

## Creating a Service

```bash
# Create a service file
sudo vim /etc/systemd/system/myapp.service
```

### Basic service:
```ini
[Unit]
Description=My Application
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=myapp
Group=myapp
WorkingDirectory=/opt/myapp
ExecStart=/opt/myapp/bin/myapp --config /etc/myapp/config.yaml
Restart=always
RestartSec=5

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/myapp /var/log/myapp

# Environment
Environment=NODE_ENV=production
EnvironmentFile=-/etc/myapp/env

# Limits
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

### Service with graceful shutdown:
```ini
[Service]
Type=simple
ExecStart=/opt/myapp/bin/myapp
ExecStop=/bin/kill -SIGTERM $MAINPID
TimeoutStopSec=30
KillMode=mixed
```

### Oneshot service (runs once):
```ini
[Service]
Type=oneshot
ExecStart=/opt/scripts/backup.sh
RemainAfterExit=no
```

```bash
# After creating/editing a service file
sudo systemctl daemon-reload
sudo systemctl enable --now myapp
```

## Timers (Cron Replacement)

### Timer file (/etc/systemd/system/backup.timer):
```ini
[Unit]
Description=Daily backup timer

[Timer]
OnCalendar=daily
# Or: OnCalendar=*-*-* 02:00:00  (every day at 2am)
# Or: OnCalendar=Mon *-*-* 09:00:00  (every Monday at 9am)
Persistent=true  # run if missed while offline
RandomizedDelaySec=300

[Install]
WantedBy=timers.target
```

### Matching service file (/etc/systemd/system/backup.service):
```ini
[Unit]
Description=Daily backup

[Service]
Type=oneshot
ExecStart=/opt/scripts/backup.sh
```

```bash
# Enable timer
sudo systemctl enable --now backup.timer

# List timers
systemctl list-timers
systemctl list-timers --all

# Run timer's service manually
sudo systemctl start backup.service

# Check timer status
systemctl status backup.timer
```

## Journal Logs (journalctl)

```bash
# View all logs
journalctl

# View logs for a service
journalctl -u myapp
journalctl -u myapp --no-pager | tail -100

# Follow logs (live)
journalctl -u myapp -f

# Last N lines
journalctl -u myapp -n 100

# Since a time
journalctl -u myapp --since "1 hour ago"
journalctl -u myapp --since "2025-01-15 10:00:00"
journalctl -u myapp --since today

# Between times
journalctl -u myapp --since "2025-01-15 10:00" --until "2025-01-15 12:00"

# Priority filter
journalctl -u myapp -p err          # errors only
journalctl -u myapp -p warning      # warnings and above

# Kernel messages
journalctl -k
journalctl -k --since "10 minutes ago"

# Disk usage
journalctl --disk-usage

# Vacuum old logs
sudo journalctl --vacuum-size=500M
sudo journalctl --vacuum-time=30d

# JSON output
journalctl -u myapp -o json-pretty -n 5
```

## System State

```bash
# Reboot / shutdown
sudo systemctl reboot
sudo systemctl poweroff

# Check boot time
systemd-analyze
systemd-analyze blame          # slow services
systemd-analyze critical-chain # dependency chain

# Default target (runlevel)
systemctl get-default
sudo systemctl set-default multi-user.target  # no GUI
sudo systemctl set-default graphical.target   # with GUI

# List failed units
systemctl --failed
sudo systemctl reset-failed  # clear failed state
```

## Debugging Failed Services

```bash
# 1. Check status
systemctl status myapp

# 2. Check recent logs
journalctl -u myapp -n 50 --no-pager

# 3. Check exit code
systemctl show myapp -p ExecMainStatus
systemctl show myapp -p ActiveState,SubState,Result

# 4. Check unit file for issues
systemctl cat myapp

# 5. Test manually
sudo -u myapp /opt/myapp/bin/myapp --config /etc/myapp/config.yaml

# 6. Check dependencies
systemctl list-dependencies myapp

# Common issues:
# - Permission denied: check User, Group, and file permissions
# - ExecStart path wrong: use absolute paths
# - Environment missing: use EnvironmentFile or Environment
# - Port already in use: check with ss -tlnp
```

## Tips

- Always run `sudo systemctl daemon-reload` after editing unit files.
- Use `Type=notify` with `sd_notify` for services that need startup confirmation.
- Use `EnvironmentFile=/etc/myapp/env` for secrets (not in unit file).
- Use `systemctl edit myapp` to create override files without modifying the original.
- Use `WantedBy=multi-user.target` for most server services.
- Use `Restart=on-failure` instead of `always` if the service shouldn't restart on clean exit.
- Use `ProtectSystem=strict` + `ReadWritePaths=` for security hardening.
```
