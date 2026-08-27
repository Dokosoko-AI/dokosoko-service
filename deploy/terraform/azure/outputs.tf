output "public_url" {
  value = local.public_url
}

output "container_app_fqdn" {
  value = azurerm_container_app.this.latest_revision_fqdn
}

output "database_fqdn" {
  value = azurerm_postgresql_flexible_server.this.fqdn
}

output "storage_account_name" {
  value = azurerm_storage_account.this.name
}
