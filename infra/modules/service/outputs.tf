output "service_name" {
  description = "ECS service name"
  value       = aws_ecs_service.this.name
}

output "task_definition_arn" {
  description = "Task definition ARN"
  value       = aws_ecs_task_definition.this.arn
}

output "service_connect_dns" {
  description = "In-cluster DNS name (via Service Connect), when a port is exposed"
  value       = var.container_port != null ? var.name : null
}
