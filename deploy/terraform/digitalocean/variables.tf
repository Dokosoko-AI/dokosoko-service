variable "name" {
  description = "Name prefix for the deployment."
  type        = string
  default     = "dokosoko"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,30}$", var.name))
    error_message = "name must be 3-31 lowercase letters, digits, or hyphens and start with a letter."
  }
}

variable "digitalocean_token" {
  description = "DigitalOcean API token. Prefer DIGITALOCEAN_TOKEN over writing this in tfvars."
  type        = string
  sensitive   = true
}

variable "region" {
  description = "DigitalOcean region slug."
  type        = string
  default     = "nyc3"
}

variable "public_url" {
  description = "Canonical HTTPS origin whose DNS points at the created load balancer."
  type        = string

  validation {
    condition     = can(regex("^https://[^/]+$", var.public_url))
    error_message = "public_url must be an HTTPS origin without a path or trailing slash."
  }
}

variable "certificate_name" {
  description = "DigitalOcean certificate name for the public hostname. Names remain stable across Let's Encrypt renewal."
  type        = string
}

variable "service_image" {
  description = "Digest-pinned image built from Dockerfile and readable by DOKS."
  type        = string

  validation {
    condition     = strcontains(var.service_image, "@sha256:")
    error_message = "service_image must be pinned by sha256 digest."
  }
}

variable "crawler_image" {
  description = "Digest-pinned image built from Dockerfile.crawler and readable by DOKS."
  type        = string

  validation {
    condition     = strcontains(var.crawler_image, "@sha256:")
    error_message = "crawler_image must be pinned by sha256 digest."
  }
}

variable "master_key" {
  description = "Base64-encoded 32-byte application master key."
  type        = string
  sensitive   = true

  validation {
    condition     = can(base64decode(var.master_key)) && length(base64decode(var.master_key)) == 32
    error_message = "master_key must be a base64-encoded 32-byte value."
  }
}

variable "setup_token" {
  description = "One-time root setup token."
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.setup_token) >= 32
    error_message = "setup_token must contain at least 32 characters."
  }
}

variable "ai_provider" {
  type    = string
  default = ""
}

variable "ai_api_key" {
  type      = string
  default   = ""
  sensitive = true
}

variable "ai_endpoint" {
  type    = string
  default = ""
}

variable "ai_model_analysis" {
  type    = string
  default = ""
}

variable "node_size" {
  description = "DOKS worker size."
  type        = string
  default     = "s-4vcpu-8gb"
}

variable "node_count" {
  description = "Worker count. The DokoSoko pod itself remains a single replica."
  type        = number
  default     = 2
}

variable "database_size" {
  description = "Managed PostgreSQL node size."
  type        = string
  default     = "db-s-2vcpu-4gb"
}

variable "storage_size" {
  description = "Block storage request for uploads and crawl snapshots."
  type        = string
  default     = "50Gi"
}

variable "upload_max_bytes" {
  type    = number
  default = 5000000
}

variable "crawler_max_pages" {
  type    = number
  default = 500
}

variable "crawler_max_bytes" {
  type    = number
  default = 5000000
}

variable "tags" {
  type    = list(string)
  default = ["dokosoko", "terraform"]
}
