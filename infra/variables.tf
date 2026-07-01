variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name for this app stack (staging or prod)"
  type        = string

  validation {
    condition     = contains(["staging", "prod"], var.environment)
    error_message = "environment must be either 'staging' or 'prod'."
  }
}

variable "project" {
  description = "Project prefix, shared with the infra repo"
  type        = string
  default     = "torque"
}

# ── Shared infra (infra repo) reference ───────────────────────────────────────

variable "tf_state_bucket" {
  description = "S3 bucket that holds the shared infra Terraform state"
  type        = string
  default     = "torque-tf-state"
}

variable "tf_locks_table" {
  description = "DynamoDB table used for Terraform state locking"
  type        = string
  default     = "torque-terraform-locks"
}

variable "shared_state_key" {
  description = "State key of the shared infra environment to consume (e.g. staging/terraform.tfstate)"
  type        = string
}

# ── Image ─────────────────────────────────────────────────────────────────────

variable "image_tag" {
  description = "Container image tag to deploy (usually the git SHA)"
  type        = string
}

# ── mqtt-guard service (internal HTTP auth webhook for EMQX) ───────────────────

variable "guard_port" {
  description = "Port the mqtt-guard HTTP server listens on (internal, Service Connect only)"
  type        = number
  default     = 8080
}

variable "guard_desired_count" {
  description = "Number of mqtt-guard tasks"
  type        = number
  default     = 1
}

variable "guard_cpu" {
  description = "mqtt-guard task CPU units"
  type        = number
  default     = 256
}

variable "guard_memory" {
  description = "mqtt-guard task memory (MiB)"
  type        = number
  default     = 512
}

# ── mqtt-listener service (internal, no inbound port) ──────────────────────────

variable "listener_desired_count" {
  description = "Number of mqtt-listener tasks"
  type        = number
  default     = 1
}

variable "listener_cpu" {
  description = "mqtt-listener task CPU units"
  type        = number
  default     = 256
}

variable "listener_memory" {
  description = "mqtt-listener task memory (MiB)"
  type        = number
  default     = 512
}

variable "mqtt_broker_url" {
  description = "MQTT broker URL the listener connects to (EMQX via Service Connect)"
  type        = string
}

# ── Application secrets (SSM parameter names in the target account) ─────────────

variable "database_url_ssm" {
  description = "SSM parameter name holding the Postgres connection string"
  type        = string
}

variable "rabbitmq_url_ssm" {
  description = "SSM parameter name holding the RabbitMQ connection string"
  type        = string
}

variable "mqtt_guard_secret_ssm" {
  description = "SSM parameter name holding the shared secret EMQX uses to call mqtt-guard"
  type        = string
}

variable "mqtt_ca_cert_ssm" {
  description = "SSM parameter name holding the MQTT CA certificate (PEM)"
  type        = string
}

variable "mqtt_cert_ssm" {
  description = "SSM parameter name holding the MQTT client certificate (PEM)"
  type        = string
}

variable "mqtt_key_ssm" {
  description = "SSM parameter name holding the MQTT client private key (PEM)"
  type        = string
}
