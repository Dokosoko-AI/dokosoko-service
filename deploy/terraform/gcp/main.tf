locals {
  required_services = toset([
    "artifactregistry.googleapis.com",
    "compute.googleapis.com",
    "file.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "sqladmin.googleapis.com",
  ])
}

resource "google_project_service" "required" {
  for_each = local.required_services

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

resource "random_id" "suffix" {
  byte_length = 4
}

resource "google_compute_network" "this" {
  name                    = var.name
  project                 = var.project_id
  auto_create_subnetworks = false

  depends_on = [google_project_service.required["compute.googleapis.com"]]
}

resource "google_compute_subnetwork" "this" {
  name          = var.name
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.this.id
  ip_cidr_range = "10.42.0.0/24"
}

resource "google_sql_database_instance" "this" {
  name                = "${var.name}-${random_id.suffix.hex}"
  project             = var.project_id
  region              = var.region
  database_version    = "POSTGRES_17"
  deletion_protection = var.deletion_protection

  settings {
    tier              = var.database_tier
    availability_type = var.database_availability_type
    disk_type         = "PD_SSD"
    disk_size         = var.database_disk_size_gb
    disk_autoresize   = true

    ip_configuration {
      ipv4_enabled = true
    }

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      transaction_log_retention_days = 7
    }

    insights_config {
      query_insights_enabled  = true
      record_application_tags = true
      record_client_address   = false
    }

    user_labels = var.labels
  }

  depends_on = [google_project_service.required["sqladmin.googleapis.com"]]
}

resource "google_sql_database" "this" {
  name     = "dokosoko"
  project  = var.project_id
  instance = google_sql_database_instance.this.name
  charset  = "UTF8"
}

resource "google_sql_user" "this" {
  name     = "dokosoko"
  project  = var.project_id
  instance = google_sql_database_instance.this.name
  password = var.database_password
}

resource "google_filestore_instance" "storage" {
  name     = var.name
  project  = var.project_id
  location = var.zone
  tier     = "BASIC_HDD"
  labels   = var.labels

  file_shares {
    capacity_gb = var.filestore_capacity_gb
    name        = "storage"
  }

  networks {
    network = google_compute_network.this.name
    modes   = ["MODE_IPV4"]
  }

  depends_on = [google_project_service.required["file.googleapis.com"]]
}

resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = substr("${var.name}-runtime", 0, 30)
  display_name = "DokoSoko runtime"

  depends_on = [google_project_service.required["iam.googleapis.com"]]
}

resource "google_project_iam_member" "cloud_sql" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

locals {
  database_url = "postgres://dokosoko:${urlencode(var.database_password)}@/${google_sql_database.this.name}?host=${urlencode("/cloudsql/${google_sql_database_instance.this.connection_name}")}&sslmode=disable"
  secret_names = toset(["database-url", "master-key", "setup-token", "ai-api-key"])
  secret_values = {
    database-url = local.database_url
    master-key   = var.master_key
    setup-token  = var.setup_token
    ai-api-key   = var.ai_api_key == "" ? "unused" : var.ai_api_key
  }
}

resource "google_secret_manager_secret" "runtime" {
  for_each = local.secret_names

  project   = var.project_id
  secret_id = "${var.name}-${each.key}"
  labels    = var.labels

  replication {
    auto {}
  }

  depends_on = [google_project_service.required["secretmanager.googleapis.com"]]
}

resource "google_secret_manager_secret_version" "runtime" {
  for_each = local.secret_names

  secret      = google_secret_manager_secret.runtime[each.key].id
  secret_data = local.secret_values[each.key]
}

resource "google_secret_manager_secret_iam_member" "runtime" {
  for_each = local.secret_names

  project   = var.project_id
  secret_id = google_secret_manager_secret.runtime[each.key].secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_cloud_run_v2_service" "this" {
  name                = var.name
  project             = var.project_id
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = var.deletion_protection
  labels              = var.labels

  template {
    service_account       = google_service_account.runtime.email
    timeout               = "300s"
    execution_environment = "EXECUTION_ENVIRONMENT_GEN2"

    scaling {
      min_instance_count = 1
      max_instance_count = 1
    }

    vpc_access {
      network_interfaces {
        network    = google_compute_network.this.name
        subnetwork = google_compute_subnetwork.this.name
        tags       = [var.name]
      }
    }

    volumes {
      name = "storage"

      nfs {
        server    = google_filestore_instance.storage.networks[0].ip_addresses[0]
        path      = "/storage"
        read_only = false
      }
    }

    volumes {
      name = "cloudsql"

      cloud_sql_instance {
        instances = [google_sql_database_instance.this.connection_name]
      }
    }

    containers {
      name  = "dokosoko"
      image = var.service_image

      ports {
        name           = "http1"
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "1Gi"
        }
        cpu_idle          = false
        startup_cpu_boost = true
      }

      env {
        name  = "DOKOSOKO_PUBLIC_URL"
        value = var.public_url
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

      dynamic "env" {
        for_each = {
          DOKOSOKO_DATABASE_URL = "database-url"
          DOKOSOKO_MASTER_KEY   = "master-key"
          DOKOSOKO_SETUP_TOKEN  = "setup-token"
          DOKOSOKO_AI_API_KEY   = "ai-api-key"
        }

        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.runtime[env.value].secret_id
              version = "latest"
            }
          }
        }
      }

      volume_mounts {
        name       = "storage"
        mount_path = "/storage"
      }
      volume_mounts {
        name       = "cloudsql"
        mount_path = "/cloudsql"
      }

      startup_probe {
        initial_delay_seconds = 0
        timeout_seconds       = 3
        period_seconds        = 5
        failure_threshold     = 30

        http_get {
          path = "/healthz"
          port = 8080
        }
      }

      liveness_probe {
        initial_delay_seconds = 10
        timeout_seconds       = 3
        period_seconds        = 15
        failure_threshold     = 3

        http_get {
          path = "/healthz"
          port = 8080
        }
      }
    }

    containers {
      name       = "crawler"
      image      = var.crawler_image
      depends_on = ["dokosoko"]

      resources {
        limits = {
          cpu    = "2"
          memory = "2Gi"
        }
        cpu_idle          = false
        startup_cpu_boost = true
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
        name = "DOKOSOKO_DATABASE_URL"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.runtime["database-url"].secret_id
            version = "latest"
          }
        }
      }

      volume_mounts {
        name       = "storage"
        mount_path = "/storage"
      }
      volume_mounts {
        name       = "cloudsql"
        mount_path = "/cloudsql"
      }
    }
  }

  depends_on = [
    google_project_iam_member.cloud_sql,
    google_secret_manager_secret_version.runtime,
    google_secret_manager_secret_iam_member.runtime,
    google_project_service.required["run.googleapis.com"],
  ]
}

resource "google_cloud_run_v2_service_iam_member" "public" {
  project  = var.project_id
  location = google_cloud_run_v2_service.this.location
  name     = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
