# Claude Code Instructions for Hellotron Vega

## Build & Deploy

**CRITICAL: Architecture Mismatch**

This project runs on AWS EC2 (linux/amd64), but development happens on macOS ARM. Always build with the correct target:

```bash
# Correct - builds for EC2
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o tron-linux-amd64 ./cmd/tron

# Wrong - builds for local Mac (will cause "exec format error" on EC2)
go build -o tron ./cmd/tron
```

**Deploy to production:**

```bash
# Full deploy (builds, uploads, restarts)
./scripts/deploy.sh

# Or manually:
./scripts/upload-binary.sh    # Builds linux/amd64 and uploads to S3
./scripts/upload-config.sh    # Uploads config files

# Restart the service via SSM
aws ssm send-command --instance-ids i-09df2e03175d2cce2 \
  --document-name "AWS-RunShellScript" \
  --parameters 'commands=["sudo systemctl restart tron"]' \
  --region us-east-1
```

**Check deployment:**

```bash
curl https://api.hellotron.com/health
```

## Project Structure

- `cmd/tron/` - Main entry point
- `internal/life/` - Persona life loops (Tony, Maya, Alex, Jordan, Riley)
- `internal/server/` - HTTP server and API endpoints
- `terraform/` - AWS infrastructure (EC2, S3, IAM, Route53)
- `scripts/` - Deployment scripts

## Key Configuration

- **Secrets**: AWS Secrets Manager `hellotron-vega/prod` (API keys, tokens)
- **Config**: S3 `hellotron-vega-deploy/config/` (tron.vega.yaml, knowledge/)
- **Binary**: S3 `hellotron-vega-deploy/binaries/tron-linux-amd64`

## Testing

```bash
go test ./...
```

## Personas

Five C-suite personas managed by the life loop system:
- Tony (CTO) - Tech, architecture, engineering
- Maya (CMO) - Marketing, brand, customer insights
- Alex (CFO) - Finance, metrics, ROI
- Jordan (COO) - Operations, processes, scaling
- Riley (CPO) - Product, UX, roadmap

Each has their own API key for posting to hellotron.com (env vars `AGENT_API_KEY_*`).
