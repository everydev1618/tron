# Hellotron Vega Terraform Configuration

This directory contains Terraform configuration for deploying Hellotron Vega to AWS.

## Architecture

```
S3: hellotron-vega-deploy/
├── binaries/tron-linux-amd64
└── config/
    ├── tron.vega.yaml
    └── knowledge/
         │
         ▼
    EC2 Instance (t3.medium)
    ├── Caddy (HTTPS reverse proxy)
    ├── Docker (for project containers)
    └── Tron service (:3000)
```

## Prerequisites

1. AWS CLI configured with credentials
2. Terraform >= 1.0
3. EC2 key pair created in AWS
4. (Optional) Route53 hosted zone for DNS

## Quick Start

```bash
# 1. Configure variables
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your values

# 2. Initialize Terraform
terraform init

# 3. Deploy
terraform apply
```

## Files

| File | Purpose |
|------|---------|
| `main.tf` | Provider configuration |
| `variables.tf` | Input variable definitions |
| `ec2.tf` | EC2 instance, security groups, IAM |
| `s3.tf` | S3 bucket for artifacts |
| `outputs.tf` | Useful output values |
| `cloud-init.yaml` | EC2 bootstrap script |
| `terraform.tfvars.example` | Example configuration |

## Required Variables

| Variable | Description |
|----------|-------------|
| `key_name` | EC2 key pair name |
| `admin_cidr_blocks` | Your IP(s) for SSH access |
| `route53_zone_id` | Route53 zone ID (if using DNS) |

## Outputs

After `terraform apply`:

```bash
terraform output public_ip       # Instance IP
terraform output webhook_url     # For VAPI/Slack config
terraform output ssh_command     # SSH connection command
terraform output ssm_command     # SSM connection command
```

## Multiple Environments

Use Terraform workspaces for staging/prod:

```bash
terraform workspace new staging
terraform workspace select staging
terraform apply -var="instance_name=staging" -var="subdomain_prefix=vega-staging"
```

## Cost

~$31/month (t3.medium + Elastic IP + minimal S3/Secrets Manager usage)
