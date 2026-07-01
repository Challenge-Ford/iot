# Reference the shared infrastructure owned by the infra repo. The cluster,
# networking and Service Connect namespace live there; this app only consumes
# their outputs so cluster-wide settings stay in a single place.
data "terraform_remote_state" "shared" {
  backend = "s3"

  config = {
    bucket         = var.tf_state_bucket
    key            = var.shared_state_key
    region         = var.aws_region
    dynamodb_table = var.tf_locks_table
    encrypt        = true
  }
}

locals {
  shared = data.terraform_remote_state.shared.outputs

  cluster_arn               = local.shared.cluster_arn
  capacity_provider_name    = local.shared.capacity_provider_name
  service_connect_namespace = local.shared.service_connect_namespace
  subnet_ids                = local.shared.subnet_ids
  security_group_id         = local.shared.security_group_id
}
