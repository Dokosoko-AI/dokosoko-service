data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  availability_zones = slice(data.aws_availability_zones.available.names, 0, 2)
  database_url       = "postgres://dokosoko:${urlencode(var.database_password)}@${aws_db_instance.this.address}:${aws_db_instance.this.port}/dokosoko?sslmode=require"

  service_environment = [
    { name = "DOKOSOKO_PUBLIC_URL", value = var.public_url },
    { name = "DOKOSOKO_DATA_DIR", value = "/storage/data" },
    { name = "DOKOSOKO_UPLOAD_DIR", value = "/storage/uploads" },
    { name = "DOKOSOKO_UPLOAD_MAX_BYTES", value = tostring(var.upload_max_bytes) },
    { name = "DOKOSOKO_AI_PROVIDER", value = var.ai_provider },
    { name = "DOKOSOKO_AI_ENDPOINT", value = var.ai_endpoint },
    { name = "DOKOSOKO_AI_MODEL_ANALYSIS", value = var.ai_model_analysis },
  ]

  crawler_environment = [
    { name = "DOKOSOKO_DATA_DIR", value = "/storage/data" },
    { name = "DOKOSOKO_UPLOAD_DIR", value = "/storage/uploads" },
    { name = "DOKOSOKO_CRAWLER_MAX_PAGES", value = tostring(var.crawler_max_pages) },
    { name = "DOKOSOKO_CRAWLER_MAX_BYTES", value = tostring(var.crawler_max_bytes) },
    { name = "DOKOSOKO_CRAWLER_ALLOW_LOCALHOST_SUBDOMAINS", value = "false" },
  ]
}

resource "aws_vpc" "this" {
  cidr_block           = "10.42.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = var.name }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = var.name }
}

resource "aws_subnet" "public" {
  for_each = { for index, zone in local.availability_zones : zone => index }

  vpc_id                  = aws_vpc.this.id
  availability_zone       = each.key
  cidr_block              = cidrsubnet(aws_vpc.this.cidr_block, 8, each.value)
  map_public_ip_on_launch = true

  tags = { Name = "${var.name}-public-${each.value + 1}" }
}

resource "aws_subnet" "app" {
  for_each = { for index, zone in local.availability_zones : zone => index }

  vpc_id            = aws_vpc.this.id
  availability_zone = each.key
  cidr_block        = cidrsubnet(aws_vpc.this.cidr_block, 8, each.value + 10)

  tags = { Name = "${var.name}-app-${each.value + 1}" }
}

resource "aws_subnet" "database" {
  for_each = { for index, zone in local.availability_zones : zone => index }

  vpc_id            = aws_vpc.this.id
  availability_zone = each.key
  cidr_block        = cidrsubnet(aws_vpc.this.cidr_block, 8, each.value + 20)

  tags = { Name = "${var.name}-database-${each.value + 1}" }
}

resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = { Name = "${var.name}-nat" }

  depends_on = [aws_internet_gateway.this]
}

resource "aws_nat_gateway" "this" {
  allocation_id = aws_eip.nat.id
  subnet_id     = values(aws_subnet.public)[0].id
  tags          = { Name = var.name }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = { Name = "${var.name}-public" }
}

resource "aws_route_table_association" "public" {
  for_each = aws_subnet.public

  subnet_id      = each.value.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "app" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.this.id
  }

  tags = { Name = "${var.name}-app" }
}

resource "aws_route_table_association" "app" {
  for_each = aws_subnet.app

  subnet_id      = each.value.id
  route_table_id = aws_route_table.app.id
}

resource "aws_security_group" "load_balancer" {
  name        = "${var.name}-load-balancer"
  description = "Public HTTPS ingress"
  vpc_id      = aws_vpc.this.id

  ingress {
    description = "HTTP redirect"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.this.cidr_block]
  }
}

resource "aws_security_group" "workload" {
  name        = "${var.name}-workload"
  description = "DokoSoko app and crawler"
  vpc_id      = aws_vpc.this.id

  ingress {
    description     = "Application traffic from the load balancer"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.load_balancer.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "database" {
  name        = "${var.name}-database"
  description = "PostgreSQL from the workload only"
  vpc_id      = aws_vpc.this.id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.workload.id]
  }
}

resource "aws_security_group" "filesystem" {
  name        = "${var.name}-filesystem"
  description = "NFS from the workload only"
  vpc_id      = aws_vpc.this.id

  ingress {
    from_port       = 2049
    to_port         = 2049
    protocol        = "tcp"
    security_groups = [aws_security_group.workload.id]
  }
}

resource "aws_db_subnet_group" "this" {
  name       = var.name
  subnet_ids = values(aws_subnet.database)[*].id
}

resource "aws_db_instance" "this" {
  identifier                 = var.name
  engine                     = "postgres"
  engine_version             = "17"
  instance_class             = var.database_instance_class
  allocated_storage          = var.database_allocated_storage_gb
  max_allocated_storage      = var.database_max_storage_gb
  storage_type               = "gp3"
  storage_encrypted          = true
  db_name                    = "dokosoko"
  username                   = "dokosoko"
  password                   = var.database_password
  port                       = 5432
  db_subnet_group_name       = aws_db_subnet_group.this.name
  vpc_security_group_ids     = [aws_security_group.database.id]
  publicly_accessible        = false
  backup_retention_period    = 7
  auto_minor_version_upgrade = true
  deletion_protection        = var.deletion_protection
  skip_final_snapshot        = !var.deletion_protection
  final_snapshot_identifier  = var.deletion_protection ? "${var.name}-final" : null
  apply_immediately          = false
}

resource "aws_efs_file_system" "this" {
  encrypted = true

  lifecycle_policy {
    transition_to_ia = "AFTER_30_DAYS"
  }

  tags = { Name = var.name }
}

resource "aws_efs_mount_target" "this" {
  for_each = aws_subnet.app

  file_system_id  = aws_efs_file_system.this.id
  subnet_id       = each.value.id
  security_groups = [aws_security_group.filesystem.id]
}

resource "aws_efs_access_point" "this" {
  file_system_id = aws_efs_file_system.this.id

  posix_user {
    uid = 65532
    gid = 65532
  }

  root_directory {
    path = "/dokosoko"

    creation_info {
      owner_uid   = 65532
      owner_gid   = 65532
      permissions = "0700"
    }
  }
}

resource "aws_secretsmanager_secret" "database_url" {
  name = "${var.name}/database-url"
}

resource "aws_secretsmanager_secret_version" "database_url" {
  secret_id     = aws_secretsmanager_secret.database_url.id
  secret_string = local.database_url
}

resource "aws_secretsmanager_secret" "master_key" {
  name = "${var.name}/master-key"
}

resource "aws_secretsmanager_secret_version" "master_key" {
  secret_id     = aws_secretsmanager_secret.master_key.id
  secret_string = var.master_key
}

resource "aws_secretsmanager_secret" "setup_token" {
  name = "${var.name}/setup-token"
}

resource "aws_secretsmanager_secret_version" "setup_token" {
  secret_id     = aws_secretsmanager_secret.setup_token.id
  secret_string = var.setup_token
}

resource "aws_secretsmanager_secret" "ai_api_key" {
  name = "${var.name}/ai-api-key"
}

resource "aws_secretsmanager_secret_version" "ai_api_key" {
  secret_id     = aws_secretsmanager_secret.ai_api_key.id
  secret_string = var.ai_api_key == "" ? "unused" : var.ai_api_key
}

resource "aws_cloudwatch_log_group" "service" {
  name              = "/ecs/${var.name}/service"
  retention_in_days = 30
}

resource "aws_cloudwatch_log_group" "crawler" {
  name              = "/ecs/${var.name}/crawler"
  retention_in_days = 30
}

data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${var.name}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "secrets" {
  statement {
    actions = ["secretsmanager:GetSecretValue"]
    resources = [
      aws_secretsmanager_secret.database_url.arn,
      aws_secretsmanager_secret.master_key.arn,
      aws_secretsmanager_secret.setup_token.arn,
      aws_secretsmanager_secret.ai_api_key.arn,
    ]
  }
}

resource "aws_iam_role_policy" "secrets" {
  name   = "runtime-secrets"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.secrets.json
}

resource "aws_iam_role" "task" {
  name               = "${var.name}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

resource "aws_ecs_cluster" "this" {
  name = var.name

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_ecs_task_definition" "service" {
  family                   = "${var.name}-service"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 512
  memory                   = 1024
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  volume {
    name = "storage"

    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.this.id
      transit_encryption = "ENABLED"

      authorization_config {
        access_point_id = aws_efs_access_point.this.id
        iam             = "DISABLED"
      }
    }
  }

  container_definitions = jsonencode([
    {
      name                   = "dokosoko"
      image                  = var.service_image
      essential              = true
      readonlyRootFilesystem = true
      user                   = "65532:65532"
      portMappings = [{
        containerPort = 8080
        hostPort      = 8080
        protocol      = "tcp"
      }]
      environment = local.service_environment
      secrets = [
        { name = "DOKOSOKO_DATABASE_URL", valueFrom = aws_secretsmanager_secret.database_url.arn },
        { name = "DOKOSOKO_MASTER_KEY", valueFrom = aws_secretsmanager_secret.master_key.arn },
        { name = "DOKOSOKO_SETUP_TOKEN", valueFrom = aws_secretsmanager_secret.setup_token.arn },
        { name = "DOKOSOKO_AI_API_KEY", valueFrom = aws_secretsmanager_secret.ai_api_key.arn },
      ]
      mountPoints = [{
        sourceVolume  = "storage"
        containerPath = "/storage"
        readOnly      = false
      }]
      linuxParameters = { initProcessEnabled = true }
      healthCheck = {
        command     = ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1"]
        interval    = 15
        timeout     = 5
        retries     = 3
        startPeriod = 20
      }
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.service.name
          awslogs-region        = var.region
          awslogs-stream-prefix = "service"
        }
      }
    }
  ])

  depends_on = [
    aws_efs_mount_target.this,
    aws_iam_role_policy.secrets,
    aws_secretsmanager_secret_version.database_url,
    aws_secretsmanager_secret_version.master_key,
    aws_secretsmanager_secret_version.setup_token,
    aws_secretsmanager_secret_version.ai_api_key,
  ]
}

resource "aws_ecs_task_definition" "crawler" {
  family                   = "${var.name}-crawler"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 1024
  memory                   = 2048
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  volume {
    name = "storage"

    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.this.id
      transit_encryption = "ENABLED"

      authorization_config {
        access_point_id = aws_efs_access_point.this.id
        iam             = "DISABLED"
      }
    }
  }

  container_definitions = jsonencode([
    {
      name                   = "crawler"
      image                  = var.crawler_image
      essential              = true
      # Fargate does not support tmpfs; Playwright needs the task's ephemeral /tmp.
      readonlyRootFilesystem = false
      user                   = "65532:65532"
      environment            = local.crawler_environment
      secrets = [
        { name = "DOKOSOKO_DATABASE_URL", valueFrom = aws_secretsmanager_secret.database_url.arn },
      ]
      mountPoints = [{
        sourceVolume  = "storage"
        containerPath = "/storage"
        readOnly      = false
      }]
      linuxParameters = { initProcessEnabled = true }
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.crawler.name
          awslogs-region        = var.region
          awslogs-stream-prefix = "crawler"
        }
      }
    }
  ])

  depends_on = [
    aws_efs_mount_target.this,
    aws_iam_role_policy.secrets,
    aws_secretsmanager_secret_version.database_url,
  ]
}

resource "aws_lb" "this" {
  name               = var.name
  load_balancer_type = "application"
  internal           = false
  security_groups    = [aws_security_group.load_balancer.id]
  subnets            = values(aws_subnet.public)[*].id
}

resource "aws_lb_target_group" "service" {
  name        = var.name
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.this.id

  health_check {
    enabled             = true
    path                = "/healthz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 15
    timeout             = 5
    matcher             = "200"
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  certificate_arn   = var.certificate_arn
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.service.arn
  }
}

resource "aws_ecs_service" "service" {
  name            = "${var.name}-service"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.service.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    assign_public_ip = false
    subnets          = values(aws_subnet.app)[*].id
    security_groups  = [aws_security_group.workload.id]
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.service.arn
    container_name   = "dokosoko"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.https]
}

resource "aws_ecs_service" "crawler" {
  name            = "${var.name}-crawler"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.crawler.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    assign_public_ip = false
    subnets          = values(aws_subnet.app)[*].id
    security_groups  = [aws_security_group.workload.id]
  }

  depends_on = [aws_ecs_service.service]
}
