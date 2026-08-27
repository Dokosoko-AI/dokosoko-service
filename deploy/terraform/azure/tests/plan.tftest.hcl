mock_provider "azurerm" {}
mock_provider "random" {}

run "production_shape" {
  command = plan

  variables {
    subscription_id   = "00000000-0000-0000-0000-000000000000"
    public_url        = "https://dokosoko.example.com"
    service_image     = "example.invalid/dokosoko@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    crawler_image     = "example.invalid/crawler@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    database_password = "test-database-password-00000000"
    master_key        = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
    setup_token       = "test-setup-token-0000000000000000"
  }

  assert {
    condition     = azurerm_container_app.this.template[0].min_replicas == 1
    error_message = "The Container App must always retain one active replica for the crawler."
  }

  assert {
    condition     = azurerm_container_app.this.template[0].max_replicas == 1
    error_message = "The Container App must not scale beyond one while storage is filesystem-backed."
  }
}
