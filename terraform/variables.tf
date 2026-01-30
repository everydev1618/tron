variable "aws_region" {
  description = "AWS region to deploy to"
  type        = string
  default     = "us-east-1"
}

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "t3.medium"
}

variable "key_name" {
  description = "Name of existing EC2 key pair for SSH access"
  type        = string
}

variable "admin_cidr_blocks" {
  description = "CIDR blocks allowed SSH access (e.g., your home IP)"
  type        = list(string)
  default     = []
}

variable "instance_name" {
  description = "Name for this instance (e.g., prod, staging). Used for subdomain and resource naming."
  type        = string
  default     = "prod"
}

variable "domain" {
  description = "Root domain name (e.g., hellotron.com)"
  type        = string
  default     = "hellotron.com"
}

variable "subdomain_prefix" {
  description = "Subdomain prefix (e.g., 'vega' results in vega.hellotron.com)"
  type        = string
  default     = "vega"
}

variable "route53_zone_id" {
  description = "Route53 hosted zone ID for the domain"
  type        = string
  default     = ""
}

variable "s3_bucket" {
  description = "S3 bucket containing binaries and configs"
  type        = string
  default     = "hellotron-vega-deploy"
}

variable "secrets_manager_secret_name" {
  description = "AWS Secrets Manager secret name containing API keys (default: hellotron-vega/prod)"
  type        = string
  default     = "hellotron-vega/prod"
}
