# Security group
resource "aws_security_group" "vega" {
  name        = "hellotron-vega-${var.instance_name}-sg"
  description = "Security group for Hellotron Vega ${var.instance_name}"

  # HTTPS for webhooks (VAPI, Slack, ElevenLabs)
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "HTTPS for webhooks"
  }

  # HTTP for Let's Encrypt ACME challenge
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "HTTP for ACME challenge"
  }

  # SSH from admin IPs only
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = var.admin_cidr_blocks
    description = "SSH access"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name    = "hellotron-vega-${var.instance_name}"
    Project = "hellotron-vega"
    Instance = var.instance_name
  }
}

# IAM role for EC2 to access S3 and Secrets Manager
resource "aws_iam_role" "vega" {
  name = "hellotron-vega-${var.instance_name}-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
    }]
  })

  tags = {
    Name    = "hellotron-vega-${var.instance_name}"
    Project = "hellotron-vega"
    Instance = var.instance_name
  }
}

resource "aws_iam_role_policy" "vega_s3" {
  name = "hellotron-vega-${var.instance_name}-s3-access"
  role = aws_iam_role.vega.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:ListBucket"
        ]
        Resource = [
          "arn:aws:s3:::${var.s3_bucket}",
          "arn:aws:s3:::${var.s3_bucket}/*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy" "vega_secrets" {
  name = "hellotron-vega-${var.instance_name}-secrets-access"
  role = aws_iam_role.vega.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue"
        ]
        Resource = [
          "arn:aws:secretsmanager:${var.aws_region}:*:secret:${var.secrets_manager_secret_name}-*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "vega_ssm" {
  role       = aws_iam_role.vega.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "vega" {
  name = "hellotron-vega-${var.instance_name}-profile"
  role = aws_iam_role.vega.name
}

# EC2 instance
resource "aws_instance" "vega" {
  ami                  = data.aws_ami.ubuntu.id
  instance_type        = var.instance_type
  key_name             = var.key_name
  iam_instance_profile = aws_iam_instance_profile.vega.name

  vpc_security_group_ids = [aws_security_group.vega.id]

  root_block_device {
    volume_size = 30
    volume_type = "gp3"
  }

  user_data = templatefile("${path.module}/cloud-init.yaml", {
    instance_name               = var.instance_name
    domain                      = var.domain
    subdomain_prefix            = var.subdomain_prefix
    fqdn                        = "${var.subdomain_prefix}.${var.domain}"
    s3_bucket                   = var.s3_bucket
    aws_region                  = var.aws_region
    secrets_manager_secret_name = var.secrets_manager_secret_name
  })

  tags = {
    Name    = "hellotron-vega-${var.instance_name}"
    Project = "hellotron-vega"
    Instance = var.instance_name
  }
}

# Elastic IP for stable address
resource "aws_eip" "vega" {
  instance = aws_instance.vega.id
  domain   = "vpc"

  tags = {
    Name    = "hellotron-vega-${var.instance_name}"
    Project = "hellotron-vega"
    Instance = var.instance_name
  }
}

# Route53 DNS record
resource "aws_route53_record" "vega" {
  count   = var.route53_zone_id != "" ? 1 : 0
  zone_id = var.route53_zone_id
  name    = "${var.subdomain_prefix}.${var.domain}"
  type    = "A"
  ttl     = 300
  records = [aws_eip.vega.public_ip]
}

# Latest Ubuntu 22.04 AMI
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}
