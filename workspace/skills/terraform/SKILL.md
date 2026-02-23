---
name: terraform
description: "Manage infrastructure as code with Terraform. Plan, apply, destroy, import resources, manage state, workspaces, and modules."
metadata: {"nanobot":{"emoji":"🟪","requires":{"bins":["terraform"]},"install":[{"id":"brew","kind":"brew","formula":"terraform","bins":["terraform"],"label":"Install Terraform (brew)"},{"id":"apt","kind":"apt","package":"terraform","bins":["terraform"],"label":"Install Terraform (apt)"}]}}
---

# Terraform Skill

Use the `terraform` CLI to manage infrastructure as code. Terraform uses HCL (HashiCorp Configuration Language) to define resources declaratively.

## Core Workflow

```bash
# Initialize (download providers, modules)
terraform init

# Format code
terraform fmt -recursive

# Validate syntax
terraform validate

# Plan (preview changes)
terraform plan

# Apply changes
terraform apply

# Apply with auto-approve (CI/CD)
terraform apply -auto-approve

# Destroy all resources
terraform destroy

# Destroy specific resource
terraform destroy -target=aws_instance.web
```

## State Management

```bash
# List resources in state
terraform state list

# Show a resource's state
terraform state show aws_instance.web

# Move a resource (rename)
terraform state mv aws_instance.old aws_instance.new

# Remove from state (without destroying)
terraform state rm aws_instance.legacy

# Pull remote state to local
terraform state pull > state.json

# Import an existing resource into state
terraform import aws_instance.web i-0abc123def456

# Refresh state (sync with real infrastructure)
terraform refresh
```

## Workspaces

```bash
# List workspaces
terraform workspace list

# Create and switch to a workspace
terraform workspace new staging
terraform workspace select staging

# Show current workspace
terraform workspace show

# Use workspace in configs:
# locals { env = terraform.workspace }
```

## Targeted Operations

```bash
# Plan only specific resources
terraform plan -target=module.vpc
terraform plan -target=aws_instance.web

# Apply only specific resources
terraform apply -target=module.database

# Replace a resource (destroy + recreate)
terraform apply -replace=aws_instance.web
```

## Variables & Outputs

```bash
# Pass variables
terraform plan -var="instance_type=t3.large"
terraform plan -var-file="prod.tfvars"

# Show outputs
terraform output
terraform output -json
terraform output db_endpoint

# Use .auto.tfvars for automatic loading
# prod.auto.tfvars is loaded automatically in the working directory
```

## Modules

```bash
# Get/update modules
terraform get
terraform init -upgrade  # upgrade providers and modules

# Common module structure:
# modules/
#   vpc/
#     main.tf
#     variables.tf
#     outputs.tf
```

## Common HCL Patterns

### Provider configuration:
```hcl
terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.0" }
  }
  backend "s3" {
    bucket = "my-terraform-state"
    key    = "prod/terraform.tfstate"
    region = "us-east-1"
  }
}

provider "aws" {
  region = var.region
}
```

### Resource with lifecycle:
```hcl
resource "aws_instance" "web" {
  ami           = var.ami_id
  instance_type = var.instance_type

  lifecycle {
    create_before_destroy = true
    prevent_destroy       = true
    ignore_changes        = [tags]
  }
}
```

### Data source:
```hcl
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }
}
```

### Dynamic blocks:
```hcl
resource "aws_security_group" "web" {
  dynamic "ingress" {
    for_each = var.ingress_rules
    content {
      from_port   = ingress.value.port
      to_port     = ingress.value.port
      protocol    = "tcp"
      cidr_blocks = ingress.value.cidrs
    }
  }
}
```

## Debugging

```bash
# Verbose logging
TF_LOG=DEBUG terraform plan

# Graph (Graphviz DOT format)
terraform graph | dot -Tpng > graph.png

# Show provider versions
terraform providers

# Lock provider versions
terraform providers lock -platform=linux_amd64 -platform=darwin_arm64
```

## Best Practices

- Always run `terraform plan` before `terraform apply`.
- Use remote state (S3, GCS, Azure Blob, Terraform Cloud) — never commit `.tfstate` files.
- Use `-target` sparingly; prefer full plans.
- Pin provider versions in `required_providers`.
- Use `terraform fmt` and `terraform validate` in CI.
- Use workspaces or directory structure to separate environments.
- When a plan shows unexpected destroys, check state with `terraform state list`.

## Tips

- Use `terraform console` for interactive expression evaluation.
- Use `terraform show` to inspect the last plan or current state.
- Use `terraform force-unlock LOCK_ID` if state gets stuck (rare).
- Use `TF_VAR_name=value` environment variables instead of `-var` in CI.
- Use `count` or `for_each` for multiple similar resources.
