variable "name" {
  description = "Name prefix for the deployment."
  type        = string
  default     = "dokosoko"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,20}$", var.name))
    error_message = "name must be 3-21 lowercase letters, digits, or hyphens and start with a letter."
  }
}

variable "subscription_id" {
  description = "Azure subscription ID."
  type        = string
}

variable "location" {
  description = "Azure region."
  type        = string
  default     = "East US"
}

variable "public_url" {
  description = "Optional canonical HTTPS origin. Empty uses the generated Container Apps hostname."
  type        = string
  default     = ""

  validation {
    condition     = var.public_url == "" || can(regex("^https://[^/]+$", var.public_url))
    error_message = "public_url must be empty or an HTTPS origin without a path or trailing slash."
  }
}

variable "service_image" {
  description = "Digest-pinned image built from Dockerfile and readable by Container Apps."
  type        = string

  validation {
    condition     = strcontains(var.service_image, "@sha256:")
    error_message = "service_image must be pinned by sha256 digest."
  }
}

variable "crawler_image" {
  description = "Digest-pinned image built from Dockerfile.crawler and readable by Container Apps."
  type        = string

  validation {
    condition     = strcontains(var.crawler_image, "@sha256:")
    error_message = "crawler_image must be pinned by sha256 digest."
  }
}

variable "database_password" {
  description = "Password for the managed DokoSoko database administrator."
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

variable "database_sku_name" {
  description = "PostgreSQL Flexible Server SKU."
  type        = string
  default     = "B_Standard_B2ms"
}

variable "database_storage_mb" {
  description = "PostgreSQL storage allocation in MiB."
  type        = number
  default     = 32768
}

variable "file_share_quota_gb" {
  description = "Azure Files quota for uploads and crawl snapshots."
  type        = number
  default     = 50
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
  type = map(string)
  default = {
    application = "dokosoko"
    managed-by  = "terraform"
  }
}
