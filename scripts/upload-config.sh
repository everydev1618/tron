#!/bin/bash
# Upload configuration files to S3 for deployment
#
# Usage: ./scripts/upload-config.sh [bucket-name]
#
# This uploads tron.vega.yaml and knowledge/ files to S3.

set -e

BUCKET="${1:-hellotron-vega-deploy}"
REGION="${AWS_REGION:-us-east-1}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "==> Uploading config files to S3..."

# Upload main config file
if [ -f "tron.vega.yaml" ]; then
    echo "    Uploading tron.vega.yaml..."
    aws s3 cp tron.vega.yaml "s3://$BUCKET/config/tron.vega.yaml" --region "$REGION"
fi

# Upload knowledge directory
if [ -d "knowledge" ]; then
    echo "    Syncing knowledge/..."
    aws s3 sync knowledge/ "s3://$BUCKET/config/knowledge/" --region "$REGION" --delete
fi

echo "==> Done! Config files uploaded to s3://$BUCKET/config/"
