variable "name" {
  description = "Name prefix for the deployment."
  type        = string
  default     = "dokosoko"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,30}$", var.name))
    error_message = "name must be 3-31 lowercase letters, digits, or hyphens and start with a letter."
  }
}

variable "region" {
  description = "AWS region."
  type        = string
}

variable "public_url" {
  description = "Canonical HTTPS origin used in links and OAuth metadata, without a trailing slash."
  type        = string

  validation {
    condition     = can(regex("^https://[^/]+$", var.public_url))
    error_message = "public_url must be an HTTPS origin without a path or trailing slash."
  }
}

variable "certificate_arn" {
  description = "ACM certificate ARN for the public hostname."
  type        = string
}

variable "service_image" {
  description = "Digest-pinned image built from Dockerfile."
  type        = string

  validation {
    condition     = strcontains(var.service_image, "@sha256:")
    error_message = "service_image must be pinned by sha256 digest."
  }
}

variable "crawler_image" {
  description = "Digest-pinned image built from Dockerfile.crawler."
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
  description = "Optional configured AI provider name."
  type        = string
  default     = ""
}

variable "ai_api_key" {
  description = "Optional AI provider credential."
  type        = string
  default     = ""
  sensitive   = true
}

variable "ai_endpoint" {
  description = "Optional AI provider endpoint override."
  type        = string
  default     = ""
}

variable "ai_model_analysis" {
  description = "Optional model override for the analysis workload."
  type        = string
  default     = ""
}

variable "upload_max_bytes" {
  description = "Maximum accepted source upload size."
  type        = number
  default     = 5000000
}

variable "crawler_max_pages" {
  description = "Maximum pages processed by one crawl."
  type        = number
  default     = 500
}

variable "crawler_max_bytes" {
  description = "Maximum bytes processed for one crawled item."
  type        = number
  default     = 5000000
}

variable "database_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.medium"
}

variable "database_allocated_storage_gb" {
  description = "Initial RDS storage allocation."
  type        = number
  default     = 50
}

variable "database_max_storage_gb" {
  description = "RDS storage autoscaling ceiling."
  type        = number
  default     = 200
}

variable "deletion_protection" {
  description = "Protect the database from accidental deletion."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags added to supported resources."
  type        = map(string)
  default = {
    Application = "dokosoko"
    ManagedBy   = "terraform"
  }
}
