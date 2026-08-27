mock_provider "google" {}
mock_provider "random" {}

run "production_shape" {
  command = plan

  variables {
    project_id        = "test-dokosoko-project"
    public_url        = "https://dokosoko.example.com"
    service_image     = "example.invalid/dokosoko@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    crawler_image     = "example.invalid/crawler@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    database_password = "test-database-password-00000000"
    master_key        = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
    setup_token       = "test-setup-token-0000000000000000"
  }

  assert {
    condition     = google_cloud_run_v2_service.this.template[0].scaling[0].min_instance_count == 1
    error_message = "Cloud Run must retain one active instance for the crawler."
  }

  assert {
    condition     = google_cloud_run_v2_service.this.template[0].scaling[0].max_instance_count == 1
    error_message = "Cloud Run must not scale beyond one while storage is filesystem-backed."
  }
}
