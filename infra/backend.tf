# Partial backend configuration. The bucket / key / region / lock table are
# supplied at `terraform init` time via -backend-config (see CI), so the same
# code targets a per-app, per-environment state key in the shared bucket:
#   backend/<environment>/terraform.tfstate
terraform {
  backend "s3" {}
}
