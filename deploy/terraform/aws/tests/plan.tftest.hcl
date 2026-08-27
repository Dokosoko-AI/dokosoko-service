mock_provider "aws" {}

override_data {
  target = data.aws_availability_zones.available
  values = {
    names = ["us-east-1a", "us-east-1b"]
  }
}

override_data {
  target = data.aws_iam_policy_document.ecs_assume_role
  values = {
    json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
  }
}

override_data {
  target = data.aws_iam_policy_document.secrets
  values = {
    json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
  }
}

run "production_shape" {
  command = plan

  variables {
    region            = "us-east-1"
    public_url        = "https://dokosoko.example.com"
    certificate_arn   = "arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000"
    service_image     = "example.invalid/dokosoko@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    crawler_image     = "example.invalid/crawler@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    database_password = "test-database-password-00000000"
    master_key        = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
    setup_token       = "test-setup-token-0000000000000000"
  }

  assert {
    condition     = aws_ecs_service.service.desired_count == 1
    error_message = "The service must remain a single active replica while storage is filesystem-backed."
  }

  assert {
    condition     = aws_ecs_service.crawler.desired_count == 1
    error_message = "The crawler must remain a single active replica while storage is filesystem-backed."
  }
}
