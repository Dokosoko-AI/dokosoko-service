output "load_balancer_dns_name" {
  description = "Create the public DNS record for this load balancer."
  value       = aws_lb.this.dns_name
}

output "public_url" {
  value = var.public_url
}

output "database_endpoint" {
  value = aws_db_instance.this.endpoint
}

output "filesystem_id" {
  value = aws_efs_file_system.this.id
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.this.name
}
