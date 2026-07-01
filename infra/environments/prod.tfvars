environment      = "prod"
shared_state_key = "prod/terraform.tfstate"

guard_desired_count = 2
guard_cpu           = 512
guard_memory        = 1024

listener_desired_count = 2
listener_cpu           = 512
listener_memory        = 1024

# EMQX broker reachable in-cluster via Service Connect.
mqtt_broker_url = "tls://torque-prod-emqx:8883"

# SSM Parameter Store names (create these in the prod account).
database_url_ssm      = "/torque/prod/iot/database_url"
rabbitmq_url_ssm      = "/torque/prod/iot/rabbitmq_url"
mqtt_guard_secret_ssm = "/torque/prod/iot/mqtt_guard_secret"
mqtt_ca_cert_ssm      = "/torque/prod/iot/mqtt_ca_cert"
mqtt_cert_ssm         = "/torque/prod/iot/mqtt_cert"
mqtt_key_ssm          = "/torque/prod/iot/mqtt_key"
