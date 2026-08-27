mock_provider "digitalocean" {}
mock_provider "kubernetes" {}

override_data {
  target = data.digitalocean_kubernetes_versions.this
  values = {
    latest_version = "1.33.1-do.0"
  }
}

run "production_shape" {
  command = plan

  variables {
    digitalocean_token = "test-token"
    public_url         = "https://dokosoko.example.com"
    certificate_name   = "dokosoko.example.com"
    service_image      = "example.invalid/dokosoko@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    crawler_image      = "example.invalid/crawler@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    master_key         = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
    setup_token        = "test-setup-token-0000000000000000"
  }

  assert {
    condition     = kubernetes_stateful_set_v1.this.spec[0].replicas == "1"
    error_message = "The app and crawler must remain in one single-replica StatefulSet."
  }
}
