variable "name" {
  description = "Name prefix for the deployment."
  type        = string
  default     = "dokosoko"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,30}$", var.name))
    error_message = "name must be 3-31 lowercase letters, digits, or hyphens and start with a letter."
  }
}

variable "project_id" {
  description = "Google Cloud project ID."
  type        = string
}

variable "region" {
  description = "Google Cloud region."
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "Zone for the deployment's Filestore instance."
  type        = string
  default     = "us-central1-a"
}

variable "public_url" {
  description = "Canonical HTTPS origin. It must route to the created Cloud Run service."
  type        = string

  validation {
    condition     = can(regex("^https://[^/]+$", var.public_url))
    error_message = "public_url must be an HTTPS origin without a path or trailing slash."
  }
}

variable "service_image" {
  description = "Digest-pinned image built from Dockerfile and readable by Cloud Run."
  type        = string

  validation {
    condition     = strcontains(var.service_image, "@sha256:")
    error_message = "service_image must be pinned by sha256 digest."
  }
}

variable "crawler_image" {
  description = "Digest-pinned image built from Dockerfile.crawler and readable by Cloud Run."
  type        = string

  validation {
    condition     = strcontains(var.crawler_image, "@sha256:")
    error_message = "crawler_image must be pinned by sha256 digest."
  }
}

variable "database_password" {
  description = "Password for the managed DokoSoko database user."
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.database_password) >= 24
    error_message = "database_password must contain at least 24 characters."
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

variable "database_tier" {
  description = "Cloud SQL machine tier."
  type        = string
  default     = "db-custom-2-7680"
}

variable "database_availability_type" {
  description = "Cloud SQL availability: ZONAL or REGIONAL."
  type        = string
  default     = "ZONAL"

  validation {
    condition     = contains(["ZONAL", "REGIONAL"], var.database_availability_type)
    error_message = "database_availability_type must be ZONAL or REGIONAL."
  }
}

variable "database_disk_size_gb" {
  type    = number
  default = 50
}

variable "filestore_capacity_gb" {
  description = "Filestore capacity for uploads and crawl snapshots. BASIC_HDD requires at least 1024 GiB."
  type        = number
  default     = 1024

  validation {
    condition     = var.filestore_capacity_gb >= 1024
    error_message = "filestore_capacity_gb must be at least 1024 for the BASIC_HDD tier."
  }
}

variable "deletion_protection" {
  type    = bool
  default = true
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

variable "labels" {
  type = map(string)
  default = {
    application = "dokosoko"
    managed_by  = "terraform"
  }
}
