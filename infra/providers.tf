provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "torque"
      Application = "iot"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}
