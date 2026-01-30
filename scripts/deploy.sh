#!/bin/bash
# Deploy Hellotron Vega to a new EC2 instance
#
# Usage: ./scripts/deploy.sh [instance-name]
#
# This script:
# 1. Uploads the binary to S3
# 2. Uploads config files to S3
# 3. Runs terraform to provision the EC2 instance
# 4. Outputs the webhook URL for configuration
#
# Prerequisites:
# - AWS credentials configured
# - Terraform initialized (terraform init in terraform/)
# - terraform.tfvars configured with base values

set -e

INSTANCE_NAME="${1:-prod}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Deploying Hellotron Vega: $INSTANCE_NAME"
echo ""

# Step 1: Upload binary
echo "==> Step 1: Building and uploading binary to S3..."
"$SCRIPT_DIR/upload-binary.sh"
echo ""

# Step 2: Upload config
echo "==> Step 2: Uploading config files to S3..."
"$SCRIPT_DIR/upload-config.sh"
echo ""

# Step 3: Run terraform
echo "==> Step 3: Provisioning EC2 instance..."
cd "$PROJECT_DIR/terraform"

# Check if terraform is initialized
if [ ! -d ".terraform" ]; then
    echo "    Running terraform init..."
    terraform init
fi

# Apply with instance name
terraform apply -var="instance_name=$INSTANCE_NAME"

echo ""
echo "==> Deployment complete!"
echo ""
echo "Outputs:"
terraform output

echo ""
echo "==> Next steps:"
echo "1. Wait 2-3 minutes for the instance to fully bootstrap"
echo "2. Check health: curl \$(terraform output -raw health_url)"
echo "3. Configure VAPI/Slack with webhook URL: $(terraform output -raw webhook_url)"
