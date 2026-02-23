---
name: pulumi
description: "Manage infrastructure as code with Pulumi using real programming languages (TypeScript, Python, Go, C#). Create, update, destroy stacks."
metadata: {"nanobot":{"emoji":"🟠","requires":{"bins":["pulumi"]},"install":[{"id":"brew","kind":"brew","formula":"pulumi","bins":["pulumi"],"label":"Install Pulumi (brew)"}]}}
---

# Pulumi Skill

Use the `pulumi` CLI to manage infrastructure with real programming languages instead of HCL. Supports TypeScript, Python, Go, C#, Java, and YAML.

## Core Workflow

```bash
# Create a new project
pulumi new aws-typescript   # or aws-python, azure-go, gcp-csharp, etc.

# Preview changes (like terraform plan)
pulumi preview

# Deploy changes
pulumi up

# Deploy with auto-approve
pulumi up --yes

# Destroy all resources
pulumi destroy

# Destroy with auto-approve
pulumi destroy --yes
```

## Stacks (Environments)

```bash
# List stacks
pulumi stack ls

# Create a new stack
pulumi stack init staging

# Select a stack
pulumi stack select production

# Show current stack
pulumi stack

# Show stack outputs
pulumi stack output
pulumi stack output vpcId

# Remove a stack
pulumi stack rm staging
```

## Configuration

```bash
# Set config values
pulumi config set aws:region us-east-1
pulumi config set instanceType t3.micro

# Set secrets (encrypted)
pulumi config set --secret dbPassword s3cret!

# Get a config value
pulumi config get instanceType

# List all config
pulumi config
```

## State & Resources

```bash
# List resources in stack
pulumi stack --show-urns

# Export state
pulumi stack export > state.json

# Import state
pulumi stack import < state.json

# Refresh state from cloud
pulumi refresh

# Import existing resource
pulumi import aws:ec2/instance:Instance web i-0abc123def456
```

## Outputs & References

```bash
# Show outputs
pulumi stack output --json

# Reference outputs from another stack
pulumi stack output vpcId --stack org/project/prod
```

## Common Patterns (TypeScript)

```typescript
import * as aws from "@pulumi/aws";

// Create a VPC
const vpc = new aws.ec2.Vpc("main", {
  cidrBlock: "10.0.0.0/16",
  tags: { Name: "main", Environment: pulumi.getStack() },
});

// Create an EC2 instance
const server = new aws.ec2.Instance("web", {
  ami: "ami-0abc123def456",
  instanceType: "t3.micro",
  subnetId: vpc.id.apply(id => ...),
  tags: { Name: "web-server" },
});

// Export outputs
export const publicIp = server.publicIp;
export const vpcId = vpc.id;
```

## Debugging

```bash
# Verbose output
pulumi up --verbose=3

# Show detailed diff
pulumi preview --diff

# Show dependency graph
pulumi stack graph graph.dot
```

## Tips

- Use `pulumi watch` for auto-deploy on file changes (dev mode).
- Use `PULUMI_CONFIG_PASSPHRASE` for local encryption without Pulumi Cloud.
- Use `pulumi convert --from terraform` to convert HCL to Pulumi.
- Use stack references to share outputs between stacks.
- Use `ComponentResource` to create reusable abstractions.
