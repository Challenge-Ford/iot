environment      = "staging"
shared_state_key = "staging/terraform.tfstate"

guard_desired_count = 1
guard_cpu           = 256
guard_memory        = 512

listener_desired_count = 1
listener_cpu           = 256
listener_memory        = 512

# EMQX broker reachable in-cluster via Service Connect.
mqtt_broker_url = "tls://torque-staging-emqx:8883"

# SSM Parameter Store names (create these in the staging account).
database_url_ssm      = "/torque/staging/iot/database_url"
rabbitmq_url_ssm      = "/torque/staging/iot/rabbitmq_url"
mqtt_guard_secret_ssm = "/torque/staging/iot/mqtt_guard_secret"
mqtt_ca_cert_ssm      = "/torque/staging/iot/mqtt_ca_cert"
mqtt_cert_ssm         = "/torque/staging/iot/mqtt_cert"
mqtt_key_ssm          = "/torque/staging/iot/mqtt_key"
