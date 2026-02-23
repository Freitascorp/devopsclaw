```skill
---
name: ansible
description: "Automate configuration management, application deployment, and orchestration with Ansible. Playbooks, inventory, roles, ad-hoc commands, and Ansible Vault."
metadata: {"nanobot":{"emoji":"🔴","requires":{"bins":["ansible"]},"install":[{"id":"brew","kind":"brew","formula":"ansible","bins":["ansible"],"label":"Install Ansible (brew)"},{"id":"pip","kind":"pip","package":"ansible","bins":["ansible"],"label":"Install Ansible (pip)"}]}}
---

# Ansible Skill

Use `ansible` and `ansible-playbook` to automate server configuration and deployments. Ansible is agentless — it connects via SSH.

## Ad-hoc Commands

```bash
# Ping all hosts
ansible all -m ping -i inventory.ini

# Run a command on all web servers
ansible webservers -a "uptime" -i inventory.ini

# Run with sudo
ansible webservers -a "systemctl restart nginx" -i inventory.ini --become

# Copy a file
ansible webservers -m copy -a "src=./app.conf dest=/etc/app/app.conf" -i inventory.ini --become

# Install a package
ansible webservers -m apt -a "name=nginx state=present" -i inventory.ini --become

# Gather facts
ansible webservers -m setup -i inventory.ini | head -100
```

## Inventory

### INI format (inventory.ini):
```ini
[webservers]
web1 ansible_host=10.0.1.10
web2 ansible_host=10.0.1.11

[dbservers]
db1 ansible_host=10.0.2.20

[production:children]
webservers
dbservers

[all:vars]
ansible_user=deploy
ansible_ssh_private_key_file=~/.ssh/deploy_key
```

### YAML format (inventory.yaml):
```yaml
all:
  children:
    webservers:
      hosts:
        web1: { ansible_host: 10.0.1.10 }
        web2: { ansible_host: 10.0.1.11 }
    dbservers:
      hosts:
        db1: { ansible_host: 10.0.2.20 }
  vars:
    ansible_user: deploy
```

## Playbooks

```bash
# Run a playbook
ansible-playbook deploy.yaml -i inventory.ini

# Dry run (check mode)
ansible-playbook deploy.yaml -i inventory.ini --check

# Verbose output
ansible-playbook deploy.yaml -i inventory.ini -vvv

# Limit to specific hosts
ansible-playbook deploy.yaml -i inventory.ini --limit web1

# Run specific tags only
ansible-playbook deploy.yaml -i inventory.ini --tags deploy,restart

# Skip certain tags
ansible-playbook deploy.yaml -i inventory.ini --skip-tags cleanup

# Pass extra variables
ansible-playbook deploy.yaml -i inventory.ini -e "version=2.1.0 env=prod"

# Step through tasks one by one
ansible-playbook deploy.yaml -i inventory.ini --step
```

### Common playbook structure:
```yaml
---
- name: Deploy web application
  hosts: webservers
  become: yes
  vars:
    app_version: "2.1.0"
    app_port: 8080

  handlers:
    - name: restart nginx
      service: name=nginx state=restarted

  tasks:
    - name: Install packages
      apt:
        name: [nginx, python3, supervisor]
        state: present
        update_cache: yes

    - name: Deploy application
      copy:
        src: "dist/app-{{ app_version }}.tar.gz"
        dest: /opt/app/
      notify: restart nginx

    - name: Configure nginx
      template:
        src: templates/nginx.conf.j2
        dest: /etc/nginx/sites-enabled/app.conf
      notify: restart nginx

    - name: Ensure service is running
      service:
        name: nginx
        state: started
        enabled: yes
```

## Roles

```bash
# Create a role scaffold
ansible-galaxy role init my-role

# Install roles from Galaxy
ansible-galaxy install geerlingguy.docker
ansible-galaxy install -r requirements.yaml

# Role directory structure:
# roles/
#   webserver/
#     tasks/main.yaml
#     handlers/main.yaml
#     templates/
#     files/
#     vars/main.yaml
#     defaults/main.yaml
```

## Ansible Vault (Secrets)

```bash
# Create an encrypted file
ansible-vault create secrets.yaml

# Edit an encrypted file
ansible-vault edit secrets.yaml

# Encrypt an existing file
ansible-vault encrypt vars/prod-secrets.yaml

# Decrypt
ansible-vault decrypt vars/prod-secrets.yaml

# View encrypted file
ansible-vault view secrets.yaml

# Run playbook with vault
ansible-playbook deploy.yaml --ask-vault-pass
ansible-playbook deploy.yaml --vault-password-file ~/.vault_pass

# Encrypt a single string
ansible-vault encrypt_string 's3cret!' --name 'db_password'
```

## Useful Modules

| Module | Purpose |
|---|---|
| `apt` / `yum` / `dnf` | Package management |
| `service` / `systemd` | Service control |
| `copy` / `template` | File management |
| `file` | File/directory permissions |
| `user` / `group` | User management |
| `git` | Clone/pull repos |
| `docker_container` | Docker management |
| `shell` / `command` | Run commands |
| `lineinfile` / `blockinfile` | Edit config files |
| `cron` | Cron jobs |
| `wait_for` | Wait for port/file |
| `uri` | HTTP requests |
| `debug` | Print variables |

## Debugging

```bash
# List tasks in a playbook
ansible-playbook deploy.yaml --list-tasks

# List hosts matched
ansible-playbook deploy.yaml --list-hosts

# Syntax check
ansible-playbook deploy.yaml --syntax-check

# Debug a specific variable
# Add to playbook: - debug: var=ansible_facts
```

## Tips

- Use `ansible.cfg` in your project root to set defaults (inventory path, remote user, etc.).
- Use `--diff` with `--check` to see what would change.
- Use `serial: 1` in playbooks for rolling updates.
- Use `delegate_to: localhost` for tasks that run on the control machine.
- Use `when:` conditionals to skip tasks: `when: ansible_os_family == "Debian"`.
- Use `register:` to capture command output for later tasks.
- Use `block:` / `rescue:` / `always:` for error handling.
```
