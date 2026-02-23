```skill
---
name: packer
description: "Build machine images with HashiCorp Packer. Create AMIs, Azure images, GCP images, Docker images, and Vagrant boxes from a single template."
metadata: {"nanobot":{"emoji":"📦","requires":{"bins":["packer"]},"install":[{"id":"brew","kind":"brew","formula":"packer","bins":["packer"],"label":"Install Packer (brew)"}]}}
---

# Packer Skill

Use `packer` to build identical machine images for multiple platforms from a single source configuration.

## Core Workflow

```bash
# Initialize (download plugins)
packer init .

# Format HCL files
packer fmt .

# Validate template
packer validate .

# Build image
packer build .

# Build with variables
packer build -var "region=us-east-1" -var "instance_type=t3.micro" .

# Build specific source only
packer build -only="amazon-ebs.ubuntu" .

# Debug mode (step through)
PACKER_LOG=1 packer build -debug .
```

## HCL Template (Modern)

### AWS AMI:
```hcl
packer {
  required_plugins {
    amazon = {
      version = ">= 1.2.0"
      source  = "github.com/hashicorp/amazon"
    }
  }
}

variable "region" {
  type    = string
  default = "us-east-1"
}

source "amazon-ebs" "ubuntu" {
  ami_name      = "myapp-{{timestamp}}"
  instance_type = "t3.micro"
  region        = var.region

  source_ami_filter {
    filters = {
      name                = "ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners      = ["099720109477"]  # Canonical
  }

  ssh_username = "ubuntu"

  tags = {
    Name    = "myapp"
    Builder = "packer"
  }
}

build {
  sources = ["source.amazon-ebs.ubuntu"]

  provisioner "shell" {
    inline = [
      "sudo apt-get update",
      "sudo apt-get install -y nginx docker.io",
      "sudo systemctl enable nginx docker",
    ]
  }

  provisioner "file" {
    source      = "config/app.conf"
    destination = "/tmp/app.conf"
  }

  provisioner "shell" {
    inline = [
      "sudo mv /tmp/app.conf /etc/myapp/app.conf",
      "sudo chown root:root /etc/myapp/app.conf",
    ]
  }

  post-processor "manifest" {
    output = "manifest.json"
  }
}
```

### Docker image:
```hcl
source "docker" "ubuntu" {
  image  = "ubuntu:22.04"
  commit = true
}

build {
  sources = ["source.docker.ubuntu"]

  provisioner "shell" {
    inline = [
      "apt-get update",
      "apt-get install -y nginx",
    ]
  }

  post-processor "docker-tag" {
    repository = "myregistry/myapp"
    tags       = ["latest", "1.0"]
  }
}
```

## Provisioners

```hcl
# Shell commands
provisioner "shell" {
  inline = ["echo hello", "sudo apt install -y nginx"]
}

# Shell script
provisioner "shell" {
  script = "scripts/setup.sh"
}

# File upload
provisioner "file" {
  source      = "files/"
  destination = "/opt/myapp/"
}

# Ansible
provisioner "ansible" {
  playbook_file = "playbook.yml"
}
```

## Multi-platform Build

```hcl
source "amazon-ebs" "base" {
  ami_name      = "myapp-aws-{{timestamp}}"
  instance_type = "t3.micro"
  region        = "us-east-1"
  source_ami    = "ami-0abc123"
  ssh_username  = "ubuntu"
}

source "azure-arm" "base" {
  subscription_id = var.azure_subscription_id
  image_publisher = "Canonical"
  image_offer     = "0001-com-ubuntu-server-jammy"
  image_sku       = "22_04-lts"
  os_type         = "Linux"
  vm_size         = "Standard_B2s"
}

source "googlecompute" "base" {
  project_id   = var.gcp_project
  source_image = "ubuntu-2204-jammy-v20240101"
  zone         = "us-central1-a"
  ssh_username = "packer"
}

build {
  sources = [
    "source.amazon-ebs.base",
    "source.azure-arm.base",
    "source.googlecompute.base",
  ]

  provisioner "shell" {
    inline = [
      "sudo apt-get update",
      "sudo apt-get install -y docker.io",
    ]
  }
}
```

## Tips

- Use `packer fmt` and `packer validate` in CI before building.
- Use `{{timestamp}}` in image names to ensure uniqueness.
- Use `packer init` to download required plugins (like `terraform init`).
- Use `-on-error=ask` during development to debug failed builds.
- Use `PACKER_LOG=1` for verbose output.
- Use `manifest` post-processor to capture output AMI IDs for Terraform.
- Build images in CI/CD on a schedule (weekly) to keep base images patched.
```
