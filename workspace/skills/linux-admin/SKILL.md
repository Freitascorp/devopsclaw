```skill
---
name: linux-admin
description: "Essential Linux system administration. Disk, memory, CPU, networking, users, processes, package management, and troubleshooting."
metadata: {"nanobot":{"emoji":"🐧","os":["linux","darwin"],"requires":{"bins":["bash"]}}}
---

# Linux Administration Skill

Essential commands for managing Linux servers. Most commands work on macOS too (noted where they differ).

## System Information

```bash
# OS info
cat /etc/os-release
uname -a
hostnamectl  # systemd systems

# Uptime and load
uptime
cat /proc/loadavg

# CPU info
nproc
lscpu
cat /proc/cpuinfo | grep "model name" | head -1

# Memory
free -h
cat /proc/meminfo | head -5

# Kernel version
uname -r
```

## Disk & Storage

```bash
# Disk usage overview
df -h
df -h /                   # specific mount

# Directory sizes
du -sh /var/log
du -sh /var/log/* | sort -rh | head -10  # top 10 largest dirs

# Find large files
find / -type f -size +100M -exec ls -lh {} \; 2>/dev/null | head -20

# Disk I/O stats
iostat -x 1 5             # 5 samples, 1s apart
iotop -oPa                # show I/O by process

# List block devices
lsblk
fdisk -l

# Check filesystem
sudo fsck -n /dev/sda1    # dry run

# Mount/unmount
sudo mount /dev/sdb1 /mnt/data
sudo umount /mnt/data
```

## Memory

```bash
# Memory usage
free -h

# Top memory consumers
ps aux --sort=-%mem | head -10

# Clear page cache (careful in prod)
sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'

# Swap usage
swapon --show
cat /proc/swaps
```

## CPU & Processes

```bash
# Process list
ps aux | head -20
ps aux --sort=-%cpu | head -10    # top CPU consumers
ps aux --sort=-%mem | head -10    # top memory consumers

# Process tree
pstree -p

# Top (interactive)
top -bn1 | head -20               # non-interactive, 1 iteration

# htop-style with sorting
top -bn1 -o %CPU | head -15

# Find a process
pgrep -la nginx
pidof nginx

# Kill a process
kill PID
kill -9 PID                        # force kill
killall nginx                      # by name
pkill -f "my-app --config"         # by pattern

# Nice / renice (priority)
nice -n 10 ./heavy-task.sh
renice -n 5 -p PID
```

## Networking

```bash
# IP addresses
ip addr show
ip a                               # short form
hostname -I                        # just IPs

# Routing table
ip route show
ip route get 8.8.8.8              # which route for a destination

# Listening ports
ss -tlnp                           # TCP
ss -ulnp                           # UDP
ss -tlnp | grep :80

# Connections
ss -tnp                            # current TCP connections
ss -s                              # connection summary

# DNS lookup
dig example.com
dig example.com +short
nslookup example.com
host example.com

# Test connectivity
ping -c 4 example.com
traceroute example.com
mtr example.com                    # interactive traceroute

# Test a port
nc -zv example.com 443
curl -sI https://example.com | head -5

# Download
curl -O https://example.com/file.tar.gz
wget https://example.com/file.tar.gz

# Firewall (iptables/nftables)
sudo iptables -L -n
sudo ufw status                    # Ubuntu
sudo firewall-cmd --list-all      # RHEL/CentOS

# Network interfaces
ip link show
ethtool eth0                       # interface details

# ARP table
ip neigh show
```

## Users & Permissions

```bash
# Current user
whoami
id

# List users
cat /etc/passwd | cut -d: -f1
getent passwd

# Add user
sudo useradd -m -s /bin/bash newuser
sudo passwd newuser

# Add to group
sudo usermod -aG docker newuser
sudo usermod -aG sudo newuser

# Switch user
su - username
sudo -u username command

# Last logins
last | head -10
lastlog | grep -v "Never"

# File permissions
ls -la
chmod 644 file.txt                 # rw-r--r--
chmod 755 script.sh                # rwxr-xr-x
chmod -R 750 /opt/myapp
chown user:group file.txt
chown -R myapp:myapp /opt/myapp

# Find files with specific permissions
find /opt -perm -o+w -type f       # world-writable files
find / -perm -4000 -type f         # SUID files
```

## Package Management

### Debian/Ubuntu (apt):
```bash
sudo apt update
sudo apt upgrade -y
sudo apt install package-name
sudo apt remove package-name
sudo apt autoremove
apt list --installed | grep nginx
dpkg -l | grep nginx
```

### RHEL/CentOS (dnf/yum):
```bash
sudo dnf update -y
sudo dnf install package-name
sudo dnf remove package-name
dnf list installed | grep nginx
rpm -qa | grep nginx
```

### Alpine (apk):
```bash
apk update
apk add package-name
apk del package-name
apk list --installed
```

## Log Files

```bash
# System logs
sudo tail -50 /var/log/syslog          # Debian/Ubuntu
sudo tail -50 /var/log/messages        # RHEL/CentOS
journalctl --since "1 hour ago"

# Auth logs
sudo tail -50 /var/log/auth.log        # Debian/Ubuntu
sudo tail -50 /var/log/secure          # RHEL/CentOS

# Kernel logs
dmesg | tail -20
dmesg -T | tail -20                    # human-readable timestamps

# Application logs
sudo tail -f /var/log/nginx/error.log

# Log rotation
sudo logrotate -f /etc/logrotate.d/myapp
```

## SSH

```bash
# Generate key
ssh-keygen -t ed25519 -C "user@hostname"

# Copy key to server
ssh-copy-id user@server

# SSH with port
ssh -p 2222 user@server

# SSH tunnel (local port forward)
ssh -L 8080:localhost:80 user@server

# SSH tunnel (remote port forward)
ssh -R 8080:localhost:3000 user@server

# SOCKS proxy
ssh -D 1080 user@server

# SSH config (~/.ssh/config)
# Host prod
#   HostName 10.0.1.10
#   User deploy
#   IdentityFile ~/.ssh/deploy_key
#   Port 22
```

## Cron

```bash
# Edit crontab
crontab -e

# List crontab
crontab -l

# System cron jobs
ls /etc/cron.d/
ls /etc/cron.daily/

# Cron format:
# ┌───── minute (0-59)
# │ ┌───── hour (0-23)
# │ │ ┌───── day of month (1-31)
# │ │ │ ┌───── month (1-12)
# │ │ │ │ ┌───── day of week (0-7, 0 and 7 = Sunday)
# * * * * * command
#
# Examples:
# 0 2 * * * /opt/scripts/backup.sh          # daily at 2am
# */5 * * * * /opt/scripts/check.sh          # every 5 minutes
# 0 9 * * 1 /opt/scripts/report.sh           # Monday at 9am
```

## Quick Diagnostics (Server Health Check)

Run this sequence when you SSH into a server to diagnose issues:

```bash
# 1. Load and uptime
uptime

# 2. Disk space
df -h | grep -E '^/dev'

# 3. Memory
free -h

# 4. Top CPU/memory processes
ps aux --sort=-%cpu | head -5

# 5. Failed services
systemctl --failed

# 6. Recent errors
journalctl -p err --since "1 hour ago" --no-pager | tail -20

# 7. Network listeners
ss -tlnp

# 8. Recent logins
last -5
```

## Tips

- Use `watch -n 5 "command"` to repeat a command every 5 seconds.
- Use `screen` or `tmux` for persistent sessions that survive disconnects.
- Use `tee` to write output to both file and stdout: `command | tee output.log`.
- Use `xargs` for batch operations: `find . -name "*.log" | xargs rm`.
- Use `lsof -i :PORT` to find what's using a port.
- Use `strace -p PID` to trace system calls (debugging).
- Use `nohup command &` to run processes that survive logout.
```
