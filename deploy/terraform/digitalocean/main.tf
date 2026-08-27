data "digitalocean_kubernetes_versions" "this" {}

resource "digitalocean_vpc" "this" {
  name     = var.name
  region   = var.region
  ip_range = "10.42.0.0/16"
}

resource "digitalocean_kubernetes_cluster" "this" {
  name          = var.name
  region        = var.region
  version       = data.digitalocean_kubernetes_versions.this.latest_version
  vpc_uuid      = digitalocean_vpc.this.id
  auto_upgrade  = true
  surge_upgrade = true
  tags          = var.tags

  node_pool {
    name       = "${var.name}-workers"
    size       = var.node_size
    node_count = var.node_count
    auto_scale = false
    tags       = var.tags
  }
}

resource "digitalocean_database_cluster" "this" {
  name                 = var.name
  engine               = "pg"
  version              = "17"
  size                 = var.database_size
  region               = var.region
  node_count           = 1
  private_network_uuid = digitalocean_vpc.this.id
  tags                 = var.tags
}

resource "digitalocean_database_db" "this" {
  cluster_id = digitalocean_database_cluster.this.id
  name       = "dokosoko"
}

resource "digitalocean_database_firewall" "this" {
  cluster_id = digitalocean_database_cluster.this.id

  rule {
    type  = "k8s"
    value = digitalocean_kubernetes_cluster.this.id
  }
}

locals {
  namespace    = "dokosoko"
  database_url = "postgres://${urlencode(digitalocean_database_cluster.this.user)}:${urlencode(digitalocean_database_cluster.this.password)}@${digitalocean_database_cluster.this.private_host}:${digitalocean_database_cluster.this.port}/${digitalocean_database_db.this.name}?sslmode=require"
}

resource "kubernetes_namespace_v1" "this" {
  metadata {
    name = local.namespace
  }
}

resource "kubernetes_secret_v1" "runtime" {
  metadata {
    name      = "runtime"
    namespace = kubernetes_namespace_v1.this.metadata[0].name
  }

  type = "Opaque"
  data = {
    database-url = local.database_url
    master-key   = var.master_key
    setup-token  = var.setup_token
    ai-api-key   = var.ai_api_key
  }
}

resource "kubernetes_stateful_set_v1" "this" {
  metadata {
    name      = var.name
    namespace = kubernetes_namespace_v1.this.metadata[0].name
    labels    = { app = var.name }
  }

  spec {
    service_name = var.name
    replicas     = 1

    selector {
      match_labels = { app = var.name }
    }

    template {
      metadata {
        labels = { app = var.name }
      }

      spec {
        security_context {
          run_as_non_root = true
          run_as_user     = 65532
          run_as_group    = 65532
          fs_group        = 65532
        }

        container {
          name              = "dokosoko"
          image             = var.service_image
          image_pull_policy = "IfNotPresent"

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem  = true

            capabilities {
              drop = ["ALL"]
            }
          }

          port {
            name           = "http"
            container_port = 8080
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
          env {
            name = "DOKOSOKO_DATABASE_URL"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.runtime.metadata[0].name
                key  = "database-url"
              }
            }
          }
          env {
            name = "DOKOSOKO_MASTER_KEY"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.runtime.metadata[0].name
                key  = "master-key"
              }
            }
          }
          env {
            name = "DOKOSOKO_SETUP_TOKEN"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.runtime.metadata[0].name
                key  = "setup-token"
              }
            }
          }
          env {
            name = "DOKOSOKO_AI_API_KEY"
            value_from {
              secret_key_ref {
                name     = kubernetes_secret_v1.runtime.metadata[0].name
                key      = "ai-api-key"
                optional = true
              }
            }
          }

          resources {
            requests = { cpu = "250m", memory = "512Mi" }
            limits   = { cpu = "1", memory = "1Gi" }
          }

          volume_mount {
            name       = "storage"
            mount_path = "/storage"
          }
          volume_mount {
            name       = "tmp"
            mount_path = "/tmp"
          }

          startup_probe {
            http_get {
              path = "/healthz"
              port = "http"
            }
            failure_threshold = 30
            period_seconds    = 5
          }

          readiness_probe {
            http_get {
              path = "/readyz"
              port = "http"
            }
            period_seconds  = 10
            timeout_seconds = 3
          }

          liveness_probe {
            http_get {
              path = "/healthz"
              port = "http"
            }
            period_seconds    = 15
            timeout_seconds   = 3
            failure_threshold = 3
          }
        }

        container {
          name              = "crawler"
          image             = var.crawler_image
          image_pull_policy = "IfNotPresent"

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem  = true

            capabilities {
              drop = ["ALL"]
            }
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
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.runtime.metadata[0].name
                key  = "database-url"
              }
            }
          }

          resources {
            requests = { cpu = "500m", memory = "1Gi" }
            limits   = { cpu = "2", memory = "3Gi" }
          }

          volume_mount {
            name       = "storage"
            mount_path = "/storage"
          }
          volume_mount {
            name       = "tmp"
            mount_path = "/tmp"
          }
          volume_mount {
            name       = "shm"
            mount_path = "/dev/shm"
          }
        }

        volume {
          name = "tmp"
          empty_dir {
            medium     = "Memory"
            size_limit = "256Mi"
          }
        }

        volume {
          name = "shm"
          empty_dir {
            medium     = "Memory"
            size_limit = "256Mi"
          }
        }
      }
    }

    volume_claim_template {
      metadata {
        name = "storage"
      }

      spec {
        access_modes       = ["ReadWriteOnce"]
        storage_class_name = "do-block-storage"

        resources {
          requests = { storage = var.storage_size }
        }
      }
    }
  }

  wait_for_rollout = true

  depends_on = [digitalocean_database_firewall.this]
}

resource "kubernetes_service_v1" "this" {
  metadata {
    name      = var.name
    namespace = kubernetes_namespace_v1.this.metadata[0].name
    annotations = {
      "service.beta.kubernetes.io/do-loadbalancer-name"                   = var.name
      "service.beta.kubernetes.io/do-loadbalancer-type"                   = "REGIONAL"
      "service.beta.kubernetes.io/do-loadbalancer-protocol"               = "http"
      "service.beta.kubernetes.io/do-loadbalancer-tls-ports"              = "443"
      "service.beta.kubernetes.io/do-loadbalancer-certificate-name"       = var.certificate_name
      "service.beta.kubernetes.io/do-loadbalancer-redirect-http-to-https" = "true"
      "service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"   = "http"
      "service.beta.kubernetes.io/do-loadbalancer-healthcheck-path"       = "/healthz"
    }
  }

  spec {
    selector = { app = var.name }
    type     = "LoadBalancer"

    port {
      name        = "http"
      port        = 80
      target_port = "http"
      protocol    = "TCP"
    }

    port {
      name        = "https"
      port        = 443
      target_port = "http"
      protocol    = "TCP"
    }
  }
}
