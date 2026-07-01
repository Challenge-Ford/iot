locals {
  expose_port = var.container_port != null

  port_mappings = local.expose_port ? [
    {
      name          = var.name
      containerPort = var.container_port
      protocol      = "tcp"
    }
  ] : []

  container_environment = [
    for k, v in var.environment_variables : { name = k, value = v }
  ]

  container_secrets = [
    for k, v in var.secrets : { name = k, valueFrom = v }
  ]
}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.name}"
  retention_in_days = var.log_retention_days
}

resource "aws_ecs_task_definition" "this" {
  family                   = var.name
  network_mode             = "awsvpc"
  requires_compatibilities = ["EC2"]
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = var.execution_role_arn
  task_role_arn            = var.task_role_arn

  container_definitions = jsonencode([
    {
      name         = var.name
      image        = var.image
      essential    = true
      portMappings = local.port_mappings
      environment  = local.container_environment
      secrets      = local.container_secrets

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.this.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "this" {
  name            = var.name
  cluster         = var.cluster_arn
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = var.desired_count

  capacity_provider_strategy {
    capacity_provider = var.capacity_provider_name
    weight            = 1
    base              = 0
  }

  network_configuration {
    subnets         = var.subnet_ids
    security_groups = [var.security_group_id]
  }

  service_connect_configuration {
    enabled   = true
    namespace = var.service_connect_namespace

    dynamic "service" {
      for_each = local.expose_port ? [1] : []
      content {
        port_name      = var.name
        discovery_name = var.name
        client_alias {
          port     = var.container_port
          dns_name = var.name
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [desired_count]
  }
}
