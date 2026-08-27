output "cloud_run_url" {
  description = "Cloud Run service URL. Ensure public_url resolves to this service."
  value       = google_cloud_run_v2_service.this.uri
}

output "public_url" {
  value = var.public_url
}

output "database_connection_name" {
  value = google_sql_database_instance.this.connection_name
}

output "filestore_address" {
  value = google_filestore_instance.storage.networks[0].ip_addresses[0]
}
