# Hellotron Vega Infrastructure

This document describes the production infrastructure setup for Hellotron Vega, including AWS configuration, deployment processes, and operational procedures.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        AWS Infrastructure                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   ┌──────────────────┐     ┌──────────────────────────────┐     │
│   │  AWS Secrets     │     │  S3 Bucket                   │     │
│   │  Manager         │     │  hellotron-vega-deploy       │     │
│   │                  │     │  ├── binaries/               │     │
│   │  hellotron-vega/ │     │  │   └── tron-linux-amd64    │     │
│   │  prod            │     │  └── config/                 │     │
│   │                  │     │      ├── tron.vega.yaml      │     │
│   │  • ANTHROPIC_KEY │     │      └── knowledge/          │     │
│   │  • VAPI_API_KEY  │     │                              │     │
│   │  • SLACK_TOKEN   │     └──────────────────────────────┘     │
│   │  • SMTP_*        │                   │                      │
│   └────────┬─────────┘                   │                      │
│            │                             │ Download on boot     │
│            │                             ▼                      │
│            │         ┌───────────────────────────────────┐      │
│            │         │  EC2 Instance (t3.medium)         │      │
│            │         │  Ubuntu 22.04 LTS                 │      │
│            │         │                                   │      │
│            │ Load    │  ┌─────────────┐  ┌────────────┐  │      │
│            │ secrets │  │   Caddy     │  │   Tron     │  │      │
│            └────────►│  │  (HTTPS)    │─►│  (:3000)   │  │      │
│                      │  │  :443/:80   │  │            │  │      │
│                      │  └─────────────┘  └────────────┘  │      │
│                      │                                   │      │
│                      │  ┌─────────────┐                  │      │
│                      │  │   Docker    │  (for projects)  │      │
│                      │  └─────────────┘                  │      │
│                      └────────────────┬──────────────────┘      │
│                                       │                         │
│                      ┌────────────────┴──────────────────┐      │
│                      │  Elastic IP + Route53             │      │
│                      │  api.hellotron.com                │      │
│                      └───────────────────────────────────┘      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘

External Connections:
├── VAPI webhooks → POST /chat/completions, /vapi/events
├── Slack events  → POST /slack/events
├── ElevenLabs    → POST /v1/elevenlabs-llm, WS /ws/elevenlabs
└── Health checks → GET /health
```

## Infrastructure Decisions

### Why EC2 (not ECS/Lambda/Fargate)?

1. **Long-running connections**: Tron handles WebSocket connections for voice and requires persistent connections
2. **Docker-in-Docker**: Agents can spawn project containers; EC2 provides full Docker access
3. **Simplicity**: Single binary deployment, no container orchestration overhead
4. **Cost**: t3.medium (~$30/month) is predictable and sufficient for this workload

### Why AWS Secrets Manager (not .env files)?

1. **Security**: No secrets in code, config files, or environment at rest
2. **Rotation**: AWS handles secret rotation without redeployment
3. **Audit**: CloudTrail logs all secret access
4. **IAM**: Fine-grained access control via IAM policies

### Why Caddy (not nginx/ALB)?

1. **Automatic HTTPS**: Let's Encrypt certificates with zero configuration
2. **Simple config**: Single Caddyfile vs complex nginx.conf
3. **Modern defaults**: HTTP/2, secure headers out of the box
4. **Low overhead**: Single binary, minimal resource usage

### Why S3 for artifacts?

1. **Durability**: 99.999999999% durability
2. **Versioning**: Automatic version history for rollbacks
3. **CI/CD integration**: Easy upload from GitHub Actions
4. **Cost**: Essentially free at our scale

## AWS Resources

| Resource | Name | Purpose |
|----------|------|---------|
| S3 Bucket | `hellotron-vega-deploy` | Binary and config storage |
| Secrets Manager | `hellotron-vega/prod` | API keys and credentials |
| EC2 Instance | `hellotron-vega-{instance}` | Application server |
| Security Group | `hellotron-vega-{instance}-sg` | Firewall rules |
| IAM Role | `hellotron-vega-{instance}-role` | EC2 permissions |
| Elastic IP | `hellotron-vega-{instance}` | Static IP for DNS |
| Route53 Record | `api.hellotron.com` | DNS A record |

## Secrets Configuration

Create a secret in AWS Secrets Manager named `hellotron-vega/prod` with this JSON structure:

```json
{
  "ANTHROPIC_API_KEY": "sk-ant-api03-...",
  "VAPI_API_KEY": "your-vapi-key",
  "VAPI_PHONE_NUMBER_ID": "your-phone-id",
  "VAPI_ASSISTANT_ID": "your-assistant-id",
  "ELEVENLABS_API_KEY": "your-elevenlabs-key",
  "ELEVENLABS_AGENT_ID": "your-agent-id",
  "SLACK_BOT_TOKEN": "xoxb-...",
  "SLACK_SIGNING_SECRET": "your-signing-secret",
  "SMTP_HOST": "smtp.example.com",
  "SMTP_PORT": "587",
  "SMTP_USER": "user@example.com",
  "SMTP_PASSWORD": "your-smtp-password",
  "SMTP_FROM": "tron@example.com"
}
```

To create the secret via CLI:

```bash
aws secretsmanager create-secret \
  --name hellotron-vega/prod \
  --description "Hellotron Vega production API keys" \
  --secret-string file://secrets.json \
  --region us-east-1
```

## Deployment

### Prerequisites

1. **AWS CLI configured** with appropriate credentials
2. **Terraform >= 1.0** installed
3. **EC2 Key Pair** created in AWS (for SSH access)
4. **Route53 Hosted Zone** for your domain (optional, for DNS)

### Initial Setup

1. **Create S3 bucket and deploy first instance:**

```bash
cd terraform

# Copy and edit variables
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your values

# Initialize Terraform
terraform init

# Create S3 bucket first (comment out EC2 resources in ec2.tf)
terraform apply -target=aws_s3_bucket.deploy -target=aws_s3_bucket_versioning.deploy \
  -target=aws_s3_bucket_server_side_encryption_configuration.deploy \
  -target=aws_s3_bucket_public_access_block.deploy

# Upload binary and config
cd ..
./scripts/upload-binary.sh
./scripts/upload-config.sh

# Deploy EC2 instance
cd terraform
terraform apply
```

2. **Or use the deploy script:**

```bash
./scripts/deploy.sh prod
```

### Updating a Running Instance

Push to the `main` branch to trigger automatic deployment via GitHub Actions.

Or manually:

```bash
# Upload new binary
./scripts/upload-binary.sh

# Upload new config
./scripts/upload-config.sh

# SSH to instance and restart
ssh -i ~/.ssh/your-key.pem ubuntu@<ip>
sudo systemctl restart tron
```

### Connecting to Instances

**Via SSH:**
```bash
ssh -i ~/.ssh/your-key.pem ubuntu@<elastic-ip>
```

**Via SSM Session Manager (no SSH required):**
```bash
./scripts/ssm.sh prod
# or
aws ssm start-session --target <instance-id> --region us-east-1
```

### Viewing Logs

```bash
# Live logs
ssh -i ~/.ssh/your-key.pem ubuntu@<ip> 'journalctl -u tron -f'

# Last 100 lines
ssh -i ~/.ssh/your-key.pem ubuntu@<ip> 'journalctl -u tron -n 100'

# Caddy access logs
ssh -i ~/.ssh/your-key.pem ubuntu@<ip> 'tail -f /var/log/caddy/access.log'
```

## Security

### Network Security

| Port | Source | Purpose |
|------|--------|---------|
| 443 | 0.0.0.0/0 | HTTPS (webhooks from VAPI, Slack, etc.) |
| 80 | 0.0.0.0/0 | Let's Encrypt ACME challenges |
| 22 | admin_cidr_blocks | SSH (restricted to your IP) |

### IAM Permissions

The EC2 instance role has minimal permissions:

- **S3**: GetObject, PutObject, ListBucket on `hellotron-vega-deploy`
- **Secrets Manager**: GetSecretValue on `hellotron-vega/prod`
- **SSM**: AmazonSSMManagedInstanceCore (for Session Manager)

### Security Best Practices

1. **SSH restricted by IP** - Only admin IPs can SSH (set in terraform.tfvars)
2. **No secrets in code** - All credentials loaded from Secrets Manager at runtime
3. **S3 bucket private** - Public access completely blocked
4. **Encryption at rest** - S3 and EBS use AES-256 encryption
5. **SSM Session Manager** - Alternative access without open SSH port

## GitHub Actions CI/CD

### Required Secrets

Configure these in GitHub repository settings:

| Secret | Description |
|--------|-------------|
| `AWS_ACCESS_KEY_ID` | IAM user access key for S3 uploads |
| `AWS_SECRET_ACCESS_KEY` | IAM user secret key |
| `EC2_SSH_KEY` | Private key for SSH deployment (PEM format) |

### Workflow Triggers

- **Push to main**: Build, upload to S3, deploy to all instances
- **Push tag v***: Build, upload versioned binary
- **Manual dispatch**: Optional instance targeting, skip deploy

### IAM User for CI/CD

Create an IAM user with these permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::hellotron-vega-deploy",
        "arn:aws:s3:::hellotron-vega-deploy/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances"
      ],
      "Resource": "*"
    }
  ]
}
```

## Cost Estimate

| Resource | Cost/Month |
|----------|------------|
| EC2 t3.medium | ~$30 |
| Elastic IP | $0 (attached) |
| S3 | ~$0.01 |
| Route53 | $0.50 |
| Secrets Manager | $0.40 |
| **Total** | **~$31/month** |

## Rollback Procedure

1. **Find previous binary version in S3:**
```bash
aws s3 ls s3://hellotron-vega-deploy/binaries/
```

2. **Copy previous version to current:**
```bash
aws s3 cp s3://hellotron-vega-deploy/binaries/tron-linux-amd64-v1.2.3 \
          s3://hellotron-vega-deploy/binaries/tron-linux-amd64
```

3. **Restart service on instance:**
```bash
ssh ubuntu@<ip> 'cd /opt/tron && \
  aws s3 cp s3://hellotron-vega-deploy/binaries/tron-linux-amd64 ./tron && \
  chmod +x ./tron && \
  sudo systemctl restart tron'
```

## Troubleshooting

### Service won't start

```bash
# Check service status
systemctl status tron

# View recent logs
journalctl -u tron -n 50 --no-pager

# Check if secrets are loading
aws secretsmanager get-secret-value \
  --secret-id hellotron-vega/prod \
  --region us-east-1
```

### Health check fails

```bash
# Test locally on instance
curl -v http://localhost:3000/health

# Check if port is listening
netstat -tlnp | grep 3000

# Check Caddy status
systemctl status caddy
journalctl -u caddy -n 50
```

### Can't SSH to instance

1. Check your IP is in `admin_cidr_blocks`
2. Use SSM Session Manager as alternative: `./scripts/ssm.sh`
3. Verify security group rules in AWS console

### Docker not working

```bash
# Check docker daemon
systemctl status docker

# Verify ubuntu user in docker group
groups ubuntu

# If not, add and reconnect
sudo usermod -aG docker ubuntu
# logout and back in
```

## Future Considerations

- **Multiple instances**: Use Terraform workspaces for staging/prod isolation
- **Auto-scaling**: Could add ASG if load increases significantly
- **Database**: If persistent state is needed, add RDS
- **Monitoring**: Add CloudWatch alarms for health checks
- **Logging**: Ship logs to CloudWatch Logs or external service
