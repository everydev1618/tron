#!/bin/bash
# Upload the tron binary to S3 for deployment
#
# Usage: ./scripts/upload-binary.sh [bucket-name]
#
# This builds the binary for linux/amd64 and uploads it to S3.

set -e

BUCKET="${1:-hellotron-vega-deploy}"
REGION="${AWS_REGION:-us-east-1}"

echo "==> Building tron for linux/amd64..."
cd "$(dirname "$0")/.."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o tron-linux-amd64 ./cmd/tron

echo "==> Uploading to s3://$BUCKET/binaries/tron-linux-amd64..."
aws s3 cp tron-linux-amd64 "s3://$BUCKET/binaries/tron-linux-amd64" --region "$REGION"

echo "==> Cleaning up local binary..."
rm tron-linux-amd64

echo "==> Done! Binary uploaded to s3://$BUCKET/binaries/tron-linux-amd64"
