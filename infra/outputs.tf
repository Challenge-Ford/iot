output "ecr_repository_urls" {
  description = "ECR repository URLs for the iot components"
  value       = module.ecr.repository_urls
}

output "mqtt_guard_service_name" {
  description = "mqtt-guard ECS service name"
  value       = module.mqtt_guard.service_name
}

output "mqtt_guard_service_connect_dns" {
  description = "In-cluster DNS name EMQX uses to reach mqtt-guard"
  value       = module.mqtt_guard.service_connect_dns
}

output "mqtt_listener_service_name" {
  description = "mqtt-listener ECS service name"
  value       = module.mqtt_listener.service_name
}
