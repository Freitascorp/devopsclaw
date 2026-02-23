---
name: aws
description: "Manage AWS infrastructure using the `aws` CLI. Covers EC2, S3, IAM, Lambda, ECS, RDS, CloudFormation, Route53, CloudWatch, and more."
metadata: {"nanobot":{"emoji":"☁️","requires":{"bins":["aws"]},"install":[{"id":"brew","kind":"brew","formula":"awscli","bins":["aws"],"label":"Install AWS CLI (brew)"},{"id":"pip","kind":"pip","package":"awscli","bins":["aws"],"label":"Install AWS CLI (pip)"}]}}
---

# AWS Cloud Skill

Use the `aws` CLI to manage Amazon Web Services resources. Always use `--output json` or `--output table` for structured output. Use `--region` when targeting a specific region.

## Authentication

```bash
# Check current identity
aws sts get-caller-identity

# Configure credentials
aws configure
aws configure --profile staging

# Switch profiles
export AWS_PROFILE=staging

# Assume a role
aws sts assume-role --role-arn arn:aws:iam::123456789012:role/MyRole --role-session-name session1
```

## EC2 — Compute

```bash
# List running instances
aws ec2 describe-instances --filters "Name=instance-state-name,Values=running" \
  --query "Reservations[].Instances[].[InstanceId,InstanceType,State.Name,PublicIpAddress,Tags[?Key=='Name']|[0].Value]" \
  --output table

# Start / stop / reboot
aws ec2 start-instances --instance-ids i-0abc123def456
aws ec2 stop-instances --instance-ids i-0abc123def456
aws ec2 reboot-instances --instance-ids i-0abc123def456

# Get instance status checks
aws ec2 describe-instance-status --instance-ids i-0abc123def456

# Launch a new instance
aws ec2 run-instances --image-id ami-0abcdef1234567890 --instance-type t3.micro \
  --key-name my-key --security-group-ids sg-0abc123 --subnet-id subnet-0abc123 \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=my-server}]'

# List security groups
aws ec2 describe-security-groups --query "SecurityGroups[].[GroupId,GroupName,Description]" --output table

# Add ingress rule
aws ec2 authorize-security-group-ingress --group-id sg-0abc123 \
  --protocol tcp --port 443 --cidr 0.0.0.0/0
```

## S3 — Storage

```bash
# List buckets
aws s3 ls

# List objects in a bucket
aws s3 ls s3://my-bucket/ --recursive --human-readable --summarize

# Copy files
aws s3 cp file.txt s3://my-bucket/path/
aws s3 cp s3://my-bucket/path/file.txt ./local/

# Sync directories
aws s3 sync ./dist/ s3://my-bucket/assets/ --delete

# Presign a URL (1 hour)
aws s3 presign s3://my-bucket/file.zip --expires-in 3600

# Bucket size
aws s3 ls s3://my-bucket/ --recursive --summarize | tail -2
```

## IAM — Identity & Access

```bash
# List users
aws iam list-users --query "Users[].[UserName,CreateDate]" --output table

# List roles
aws iam list-roles --query "Roles[].[RoleName,Arn]" --output table

# Get attached policies for a role
aws iam list-attached-role-policies --role-name MyRole

# Create a user
aws iam create-user --user-name deploy-bot
aws iam create-access-key --user-name deploy-bot

# Attach a managed policy
aws iam attach-user-policy --user-name deploy-bot \
  --policy-arn arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess
```

## ECS — Containers

```bash
# List clusters
aws ecs list-clusters

# List services in a cluster
aws ecs list-services --cluster my-cluster

# Describe a service
aws ecs describe-services --cluster my-cluster --services my-service \
  --query "services[].[serviceName,status,runningCount,desiredCount]" --output table

# Force new deployment (rolling restart)
aws ecs update-service --cluster my-cluster --service my-service --force-new-deployment

# List tasks
aws ecs list-tasks --cluster my-cluster --service-name my-service

# Exec into a running container
aws ecs execute-command --cluster my-cluster --task TASK_ARN \
  --container my-container --interactive --command "/bin/sh"

# View task logs
aws logs get-log-events --log-group-name /ecs/my-service \
  --log-stream-name ecs/my-container/TASK_ID --limit 50
```

## Lambda — Serverless

```bash
# List functions
aws lambda list-functions --query "Functions[].[FunctionName,Runtime,MemorySize,LastModified]" --output table

# Invoke a function
aws lambda invoke --function-name my-func --payload '{"key":"value"}' /dev/stdout

# Update function code
aws lambda update-function-code --function-name my-func --zip-file fileb://function.zip

# View recent invocations
aws logs filter-log-events --log-group-name /aws/lambda/my-func --limit 20
```

## RDS — Databases

```bash
# List DB instances
aws rds describe-db-instances \
  --query "DBInstances[].[DBInstanceIdentifier,DBInstanceClass,Engine,DBInstanceStatus]" --output table

# Create a snapshot
aws rds create-db-snapshot --db-instance-identifier my-db --db-snapshot-identifier my-db-snap-$(date +%Y%m%d)

# List snapshots
aws rds describe-db-snapshots --db-instance-identifier my-db \
  --query "DBSnapshots[].[DBSnapshotIdentifier,Status,SnapshotCreateTime]" --output table

# Reboot instance
aws rds reboot-db-instance --db-instance-identifier my-db
```

## CloudFormation — IaC

```bash
# List stacks
aws cloudformation list-stacks --stack-status-filter CREATE_COMPLETE UPDATE_COMPLETE \
  --query "StackSummaries[].[StackName,StackStatus,CreationTime]" --output table

# Deploy a stack
aws cloudformation deploy --template-file template.yaml --stack-name my-stack \
  --parameter-overrides Env=prod --capabilities CAPABILITY_IAM

# Stack events (troubleshoot failures)
aws cloudformation describe-stack-events --stack-name my-stack \
  --query "StackEvents[?ResourceStatus=='CREATE_FAILED'].[LogicalResourceId,ResourceStatusReason]" --output table

# Delete a stack
aws cloudformation delete-stack --stack-name my-stack
```

## Route 53 — DNS

```bash
# List hosted zones
aws route53 list-hosted-zones --query "HostedZones[].[Id,Name,ResourceRecordSetCount]" --output table

# List records in a zone
aws route53 list-resource-record-sets --hosted-zone-id Z0123456789 \
  --query "ResourceRecordSets[].[Name,Type,TTL]" --output table
```

## CloudWatch — Monitoring

```bash
# Get CPU utilization for an instance (last 1 hour, 5-min intervals)
aws cloudwatch get-metric-statistics --namespace AWS/EC2 \
  --metric-name CPUUtilization --dimensions Name=InstanceId,Value=i-0abc123 \
  --start-time $(date -u -v-1H +%Y-%m-%dT%H:%M:%S) --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 --statistics Average

# List alarms
aws cloudwatch describe-alarms --state-value ALARM \
  --query "MetricAlarms[].[AlarmName,StateValue,MetricName]" --output table

# Tail CloudWatch logs
aws logs tail /ecs/my-service --follow --since 10m
```

## Cost & Billing

```bash
# Get cost for the current month
aws ce get-cost-and-usage \
  --time-period Start=$(date +%Y-%m-01),End=$(date +%Y-%m-%d) \
  --granularity MONTHLY --metrics BlendedCost \
  --group-by Type=DIMENSION,Key=SERVICE
```

## Tips

- Always use `--query` with JMESPath to filter output — it's faster and cleaner than piping to `jq`.
- Use `--dry-run` on EC2 commands to validate permissions without executing.
- Use `aws configure list` to debug which credentials are active.
- Use `--no-paginate` or `--max-items` to control output size.
- For large outputs, pipe through `| head -50` to avoid flooding the terminal.
