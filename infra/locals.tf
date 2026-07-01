data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

locals {
  name_prefix  = "${var.project}-iot-${var.environment}"
  ecr_registry = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${data.aws_region.current.name}.amazonaws.com"

  components = ["mqtt-guard", "mqtt-listener"]

  # SSM parameter names holding application secrets.
  secret_ssm_names = {
    database_url      = var.database_url_ssm
    rabbitmq_url      = var.rabbitmq_url_ssm
    mqtt_guard_secret = var.mqtt_guard_secret_ssm
    mqtt_ca_cert      = var.mqtt_ca_cert_ssm
    mqtt_cert         = var.mqtt_cert_ssm
    mqtt_key          = var.mqtt_key_ssm
  }

  # Full ARNs of those parameters, used by the container `secrets` block.
  secret_arns = {
    for k, name in local.secret_ssm_names :
    k => "arn:aws:ssm:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:parameter${name}"
  }
}
