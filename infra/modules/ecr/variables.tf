variable "repository_names" {
  description = "ECR repository names to create"
  type        = list(string)
}

variable "keep_last_images" {
  description = "Number of most recent images to retain"
  type        = number
  default     = 20
}
