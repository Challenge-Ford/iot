variable "name" {
  description = "Service name (also the Service Connect / task family name)"
  type        = string
}

variable "cluster_arn" {
  description = "ARN of the shared ECS cluster"
  type        = string
}

variable "capacity_provider_name" {
  description = "Capacity provider used by the shared cluster"
  type        = string
}

variable "subnet_ids" {
  description = "Subnets for the awsvpc task ENIs"
  type        = list(string)
}

variable "security_group_id" {
  description = "Security group applied to the task ENIs"
  type        = string
}

variable "execution_role_arn" {
  description = "ECS task execution role ARN (image pull, logs, SSM secrets)"
  type        = string
}

variable "task_role_arn" {
  description = "ECS task role ARN (application AWS permissions)"
  type        = string
}

variable "image" {
  description = "Fully-qualified container image (repo:tag)"
  type        = string
}

variable "cpu" {
  description = "Task CPU units"
  type        = number
}

variable "memory" {
  description = "Task memory (MiB)"
  type        = number
}

variable "desired_count" {
  description = "Number of tasks"
  type        = number
  default     = 1
}

variable "aws_region" {
  description = "AWS region (for the log driver)"
  type        = string
}

variable "environment_variables" {
  description = "Plain environment variables for the container"
  type        = map(string)
  default     = {}
}

variable "secrets" {
  description = "Secrets injected from SSM Parameter Store: env var name => parameter ARN"
  type        = map(string)
  default     = {}
}

variable "service_connect_namespace" {
  description = "Service Connect namespace (name or ARN) from the shared cluster"
  type        = string
}

variable "container_port" {
  description = "Container port to expose via Service Connect. null for workers with no inbound port."
  type        = number
  default     = null
}

variable "log_retention_days" {
  description = "CloudWatch log retention"
  type        = number
  default     = 30
}
