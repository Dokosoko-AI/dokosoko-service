resource "random_id" "suffix" {
  byte_length = 4
}

locals {
  compact_name = replace(var.name, "-", "")
  suffix       = random_id.suffix.hex
  server_name  = "${var.name}-pg-${local.suffix}"
}

resource "azurerm_resource_group" "this" {
  name     = var.name
  location = var.location
  tags     = var.tags
}

resource "azurerm_virtual_network" "this" {
  name                = var.name
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  address_space       = ["10.42.0.0/16"]
  tags                = var.tags
}

resource "azurerm_subnet" "container_apps" {
  name                 = "container-apps"
  resource_group_name  = azurerm_resource_group.this.name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = ["10.42.0.0/23"]

  delegation {
    name = "container-apps"

    service_delegation {
      name    = "Microsoft.App/environments"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

resource "azurerm_subnet" "database" {
  name                 = "database"
  resource_group_name  = azurerm_resource_group.this.name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = ["10.42.2.0/24"]
  service_endpoints    = ["Microsoft.Storage"]

  delegation {
    name = "postgresql"

    service_delegation {
      name    = "Microsoft.DBforPostgreSQL/flexibleServers"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

resource "azurerm_private_dns_zone" "database" {
  name                = "${var.name}.postgres.database.azure.com"
  resource_group_name = azurerm_resource_group.this.name
  tags                = var.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "database" {
  name                  = var.name
  private_dns_zone_name = azurerm_private_dns_zone.database.name
  virtual_network_id    = azurerm_virtual_network.this.id
  resource_group_name   = azurerm_resource_group.this.name
}

resource "azurerm_postgresql_flexible_server" "this" {
  name                          = local.server_name
  resource_group_name           = azurerm_resource_group.this.name
  location                      = azurerm_resource_group.this.location
  version                       = "17"
  delegated_subnet_id           = azurerm_subnet.database.id
  private_dns_zone_id           = azurerm_private_dns_zone.database.id
  public_network_access_enabled = false
  administrator_login           = "dokosoko_admin"
  administrator_password        = var.database_password
  sku_name                      = var.database_sku_name
  storage_mb                    = var.database_storage_mb
  storage_tier                  = "P4"
  backup_retention_days         = 7
  geo_redundant_backup_enabled  = false
  tags                          = var.tags

  lifecycle {
    prevent_destroy = true
  }

  depends_on = [azurerm_private_dns_zone_virtual_network_link.database]
}

resource "azurerm_postgresql_flexible_server_configuration" "extensions" {
  name      = "azure.extensions"
  server_id = azurerm_postgresql_flexible_server.this.id
  value     = "VECTOR,CITEXT,PGCRYPTO"
}

resource "azurerm_postgresql_flexible_server_database" "this" {
  name      = "dokosoko"
  server_id = azurerm_postgresql_flexible_server.this.id
  charset   = "UTF8"
  collation = "en_US.utf8"

  depends_on = [azurerm_postgresql_flexible_server_configuration.extensions]
}

resource "azurerm_log_analytics_workspace" "this" {
  name                = var.name
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  sku                 = "PerGB2018"
  retention_in_days   = 30
  tags                = var.tags
}

resource "azurerm_container_app_environment" "this" {
  name                       = var.name
  location                   = azurerm_resource_group.this.location
  resource_group_name        = azurerm_resource_group.this.name
  log_analytics_workspace_id = azurerm_log_analytics_workspace.this.id
  infrastructure_subnet_id   = azurerm_subnet.container_apps.id
  tags                       = var.tags
}

resource "azurerm_storage_account" "this" {
  name                            = substr("${local.compact_name}${local.suffix}", 0, 24)
  resource_group_name             = azurerm_resource_group.this.name
  location                        = azurerm_resource_group.this.location
  account_tier                    = "Standard"
  account_replication_type        = "ZRS"
  min_tls_version                 = "TLS1_2"
  shared_access_key_enabled       = true
  allow_nested_items_to_be_public = false
  tags                            = var.tags
}

resource "azurerm_storage_share" "this" {
  name               = "storage"
  storage_account_id = azurerm_storage_account.this.id
  quota              = var.file_share_quota_gb
  enabled_protocol   = "SMB"
}

resource "azurerm_container_app_environment_storage" "this" {
  name                         = "storage"
  container_app_environment_id = azurerm_container_app_environment.this.id
  account_name                 = azurerm_storage_account.this.name
  share_name                   = azurerm_storage_share.this.name
  access_key                   = azurerm_storage_account.this.primary_access_key
  access_mode                  = "ReadWrite"
}

locals {
  public_url   = var.public_url == "" ? "https://${var.name}.${azurerm_container_app_environment.this.default_domain}" : var.public_url
  database_url = "postgres://dokosoko_admin:${urlencode(var.database_password)}@${azurerm_postgresql_flexible_server.this.fqdn}:5432/${azurerm_postgresql_flexible_server_database.this.name}?sslmode=require"
}

resource "azurerm_container_app" "this" {
  name                         = var.name
  container_app_environment_id = azurerm_container_app_environment.this.id
  resource_group_name          = azurerm_resource_group.this.name
  revision_mode                = "Single"
  tags                         = var.tags

  secret {
    name  = "database-url"
    value = local.database_url
  }
  secret {
    name  = "master-key"
    value = var.master_key
  }
  secret {
    name  = "setup-token"
    value = var.setup_token
  }
  secret {
    name  = "ai-api-key"
    value = var.ai_api_key == "" ? "unused" : var.ai_api_key
  }

  template {
    min_replicas = 1
    max_replicas = 1

    volume {
      name         = "storage"
      storage_name = azurerm_container_app_environment_storage.this.name
      storage_type = "AzureFile"
    }

    container {
      name   = "dokosoko"
      image  = var.service_image
      cpu    = 0.5
      memory = "1Gi"

      env {
        name  = "DOKOSOKO_PUBLIC_URL"
        value = local.public_url
      }
      env {
        name  = "DOKOSOKO_DATA_DIR"
        value = "/storage/data"
      }
      env {
        name  = "DOKOSOKO_UPLOAD_DIR"
        value = "/storage/uploads"
      }
      env {
        name  = "DOKOSOKO_UPLOAD_MAX_BYTES"
        value = tostring(var.upload_max_bytes)
      }
      env {
        name  = "DOKOSOKO_AI_PROVIDER"
        value = var.ai_provider
      }
      env {
        name  = "DOKOSOKO_AI_ENDPOINT"
        value = var.ai_endpoint
      }
      env {
        name  = "DOKOSOKO_AI_MODEL_ANALYSIS"
        value = var.ai_model_analysis
      }
      env {
        name        = "DOKOSOKO_DATABASE_URL"
        secret_name = "database-url"
      }
      env {
        name        = "DOKOSOKO_MASTER_KEY"
        secret_name = "master-key"
      }
      env {
        name        = "DOKOSOKO_SETUP_TOKEN"
        secret_name = "setup-token"
      }
      env {
        name        = "DOKOSOKO_AI_API_KEY"
        secret_name = "ai-api-key"
      }

      volume_mounts {
        name = "storage"
        path = "/storage"
      }

      startup_probe {
        transport               = "HTTP"
        port                    = 8080
        path                    = "/healthz"
        interval_seconds        = 5
        timeout                 = 3
        failure_count_threshold = 30
      }

      readiness_probe {
        transport               = "HTTP"
        port                    = 8080
        path                    = "/readyz"
        interval_seconds        = 10
        timeout                 = 3
        failure_count_threshold = 3
      }

      liveness_probe {
        transport               = "HTTP"
        port                    = 8080
        path                    = "/healthz"
        interval_seconds        = 15
        timeout                 = 3
        failure_count_threshold = 3
      }
    }

    container {
      name   = "crawler"
      image  = var.crawler_image
      cpu    = 1
      memory = "2Gi"

      env {
        name  = "DOKOSOKO_DATA_DIR"
        value = "/storage/data"
      }
      env {
        name  = "DOKOSOKO_UPLOAD_DIR"
        value = "/storage/uploads"
      }
      env {
        name  = "DOKOSOKO_CRAWLER_MAX_PAGES"
        value = tostring(var.crawler_max_pages)
      }
      env {
        name  = "DOKOSOKO_CRAWLER_MAX_BYTES"
        value = tostring(var.crawler_max_bytes)
      }
      env {
        name  = "DOKOSOKO_CRAWLER_ALLOW_LOCALHOST_SUBDOMAINS"
        value = "false"
      }
      env {
        name        = "DOKOSOKO_DATABASE_URL"
        secret_name = "database-url"
      }

      volume_mounts {
        name = "storage"
        path = "/storage"
      }
    }
  }

  ingress {
    external_enabled           = true
    allow_insecure_connections = false
    target_port                = 8080
    transport                  = "http"

    traffic_weight {
      latest_revision = true
      percentage      = 100
    }
  }

  depends_on = [
    azurerm_postgresql_flexible_server_database.this,
    azurerm_container_app_environment_storage.this,
  ]
}
