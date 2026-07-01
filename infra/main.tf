# ── IAM ─────────────────────────────────────────────────────────────────────

data "aws_iam_policy_document" "ecs_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${local.name_prefix}-exec"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "secrets_read" {
  statement {
    effect    = "Allow"
    actions   = ["ssm:GetParameters", "ssm:GetParameter"]
    resources = [for name in local.secret_ssm_names : "arn:aws:ssm:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:parameter${name}"]
  }
}

resource "aws_iam_role_policy" "execution_secrets" {
  name   = "${local.name_prefix}-secrets"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.secrets_read.json
}

resource "aws_iam_role" "task" {
  name               = "${local.name_prefix}-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

# ── ECR ────────────────────────────────────────────────────────────────────────

module "ecr" {
  source           = "./modules/ecr"
  repository_names = [for c in local.components : "${var.project}-iot-${c}"]
}

# ── Services ───────────────────────────────────────────────────────────────────

# Internal HTTP auth webhook called by EMQX over Service Connect. Not exposed
# to the internet.
module "mqtt_guard" {
  source = "./modules/service"

  name                   = "${local.name_prefix}-mqtt-guard"
  cluster_arn            = local.cluster_arn
  capacity_provider_name = local.capacity_provider_name
  subnet_ids             = local.subnet_ids
  security_group_id      = local.security_group_id
  execution_role_arn     = aws_iam_role.execution.arn
  task_role_arn          = aws_iam_role.task.arn

  image          = "${local.ecr_registry}/${var.project}-iot-mqtt-guard:${var.image_tag}"
  cpu            = var.guard_cpu
  memory         = var.guard_memory
  desired_count  = var.guard_desired_count
  container_port = var.guard_port
  aws_region     = data.aws_region.current.name

  service_connect_namespace = local.service_connect_namespace

  environment_variables = {
    APP_ENV  = var.environment
    PORT     = tostring(var.guard_port)
    LOG_JSON = "true"
  }

  secrets = {
    DATABASE_URL      = local.secret_arns["database_url"]
    MQTT_GUARD_SECRET = local.secret_arns["mqtt_guard_secret"]
  }
}

# Internal MQTT subscriber that forwards telemetry to RabbitMQ. No inbound port.
module "mqtt_listener" {
  source = "./modules/service"

  name                   = "${local.name_prefix}-mqtt-listener"
  cluster_arn            = local.cluster_arn
  capacity_provider_name = local.capacity_provider_name
  subnet_ids             = local.subnet_ids
  security_group_id      = local.security_group_id
  execution_role_arn     = aws_iam_role.execution.arn
  task_role_arn          = aws_iam_role.task.arn

  image         = "${local.ecr_registry}/${var.project}-iot-mqtt-listener:${var.image_tag}"
  cpu           = var.listener_cpu
  memory        = var.listener_memory
  desired_count = var.listener_desired_count
  aws_region    = data.aws_region.current.name

  service_connect_namespace = local.service_connect_namespace

  environment_variables = {
    APP_ENV         = var.environment
    LOG_JSON        = "true"
    MQTT_BROKER_URL = var.mqtt_broker_url
  }

  secrets = {
    DATABASE_URL = local.secret_arns["database_url"]
    RABBITMQ_URL = local.secret_arns["rabbitmq_url"]
    MQTT_CA_CERT = local.secret_arns["mqtt_ca_cert"]
    MQTT_CERT    = local.secret_arns["mqtt_cert"]
    MQTT_KEY     = local.secret_arns["mqtt_key"]
  }
}
