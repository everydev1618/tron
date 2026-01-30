#!/bin/bash
# Connect to EC2 instance via SSM Session Manager
#
# Usage: ./scripts/ssm.sh [instance-name]
#
# This is an alternative to SSH that doesn't require open ports.

set -e

INSTANCE_NAME="${1:-prod}"
REGION="${AWS_REGION:-us-east-1}"

# Get instance ID by tag
INSTANCE_ID=$(aws ec2 describe-instances \
    --filters "Name=tag:Project,Values=hellotron-vega" "Name=tag:Instance,Values=$INSTANCE_NAME" "Name=instance-state-name,Values=running" \
    --query 'Reservations[0].Instances[0].InstanceId' \
    --output text \
    --region "$REGION")

if [ "$INSTANCE_ID" == "None" ] || [ -z "$INSTANCE_ID" ]; then
    echo "Error: No running instance found with name '$INSTANCE_NAME'"
    exit 1
fi

echo "Connecting to $INSTANCE_NAME ($INSTANCE_ID)..."
aws ssm start-session --target "$INSTANCE_ID" --region "$REGION"
