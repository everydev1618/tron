output "public_ip" {
  description = "Elastic IP address for the instance"
  value       = aws_eip.vega.public_ip
}

output "instance_id" {
  description = "EC2 instance ID"
  value       = aws_instance.vega.id
}

output "fqdn" {
  description = "Fully qualified domain name"
  value       = "${var.subdomain_prefix}.${var.domain}"
}

output "webhook_url" {
  description = "URL for VAPI/Slack webhook configuration"
  value       = "https://${var.subdomain_prefix}.${var.domain}/chat/completions"
}

output "health_url" {
  description = "Health check URL"
  value       = "https://${var.subdomain_prefix}.${var.domain}/health"
}

output "ssh_command" {
  description = "SSH command to connect to the instance"
  value       = "ssh -i ~/.ssh/${var.key_name}.pem ubuntu@${aws_eip.vega.public_ip}"
}

output "logs_command" {
  description = "Command to view tron logs"
  value       = "ssh -i ~/.ssh/${var.key_name}.pem ubuntu@${aws_eip.vega.public_ip} 'journalctl -u tron -f'"
}

output "ssm_command" {
  description = "SSM Session Manager command to connect to the instance"
  value       = "aws ssm start-session --target ${aws_instance.vega.id} --region ${var.aws_region}"
}
