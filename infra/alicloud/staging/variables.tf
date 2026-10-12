variable "region" {
  description = "Alibaba Cloud region selected in the console after checking ECS and OSS availability."
  type        = string
}

variable "zone_id" {
  description = "An available zone in region with the selected x86_64 ECS instance type."
  type        = string
}

variable "image_id" {
  description = "Exact Ubuntu 24.04 x86_64 image ID selected and reviewed in the target region."
  type        = string
}

variable "ssh_key_name" {
  description = "Pre-created/imported public key-pair name. The private key stays with the operator."
  type        = string
}

variable "ssh_cidr" {
  description = "Operator public IPv4 CIDR, normally a single /32. Empty means no public SSH rule and no public bandwidth."
  type        = string
  default     = ""

  validation {
    condition     = var.ssh_cidr == "" || (can(cidrnetmask(var.ssh_cidr)) && try(split("/", var.ssh_cidr)[1], "") == "32")
    error_message = "ssh_cidr must be empty or one explicit IPv4 /32 address. Broad SSH ranges are forbidden."
  }
}

variable "instance_type" {
  description = "Target x86_64 ECS instance type; availability and exact price must be confirmed in the account console."
  type        = string
  default     = "ecs.g7.xlarge"
}

variable "worker_b_instance_type" {
  description = "Separately selected lower-cost ECS-B type; required only when create_worker_b=true and must be reviewed for DB-only Worker load."
  type        = string
  default     = ""

  validation {
    condition     = !var.create_worker_b || (var.worker_b_instance_type != "" && var.worker_b_instance_type != var.instance_type)
    error_message = "When create_worker_b is true, worker_b_instance_type must be explicitly set and differ from the core instance_type."
  }
}

variable "system_disk_size_gib" {
  description = "System disk size. Confirm regional minimums and price before planning."
  type        = number
  default     = 40

  validation {
    condition     = var.system_disk_size_gib >= 40 && var.system_disk_size_gib <= 200
    error_message = "system_disk_size_gib must be between 40 and 200 GiB."
  }
}

variable "create_worker_b" {
  description = "Create the second ECS only for the separately approved multi-host acceptance stage."
  type        = bool
  default     = false
}

variable "run_id" {
  description = "Unique ownership token included in resource tags and remote evidence paths."
  type        = string

  validation {
    condition     = can(regex("^[a-zA-Z0-9][a-zA-Z0-9-]{7,39}$", var.run_id))
    error_message = "run_id must be 8-40 alphanumeric/hyphen characters and begin with an alphanumeric character."
  }
}

variable "resource_ttl" {
  description = "UTC cleanup deadline tag, for example 2026-11-01T00:00:00Z. A tag is not an automatic expiry mechanism."
  type        = string

  validation {
    condition     = can(regex("^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$", var.resource_ttl))
    error_message = "resource_ttl must be an explicit UTC timestamp ending in Z."
  }
}

variable "enable_resources" {
  description = "Explicit cost-bearing resource creation gate. Keep false until plan and cost review are approved."
  type        = bool
  default     = false
}

variable "budget_approved" {
  description = "Separate human approval gate after reviewing region-specific ECS, disk, network, OSS, and snapshot charges."
  type        = bool
  default     = false
}

variable "vpc_cidr" {
  description = "Run-scoped IPv4 VPC network. Must contain the selected vSwitch CIDR."
  type        = string
  default     = "10.77.0.0/16"

  validation {
    condition     = can(cidrnetmask(var.vpc_cidr))
    error_message = "vpc_cidr must be a valid IPv4 CIDR."
  }
}

variable "vswitch_cidr" {
  description = "Run-scoped IPv4 subnet fully contained in vpc_cidr."
  type        = string
  default     = "10.77.1.0/24"

  validation {
    condition = can(cidrnetmask(var.vswitch_cidr)) && try(
      tonumber(split("/", var.vswitch_cidr)[1]) >= tonumber(split("/", var.vpc_cidr)[1]) &&
      tonumber(split("/", var.vswitch_cidr)[1]) - tonumber(split("/", var.vpc_cidr)[1]) <= 8 &&
      contains(
        [for subnet_index in range(0, pow(2, tonumber(split("/", var.vswitch_cidr)[1]) - tonumber(split("/", var.vpc_cidr)[1]))) : cidrsubnet(var.vpc_cidr, tonumber(split("/", var.vswitch_cidr)[1]) - tonumber(split("/", var.vpc_cidr)[1]), subnet_index)],
        var.vswitch_cidr
      ),
      false
    )
    error_message = "vswitch_cidr must be a valid IPv4 subnet contained in vpc_cidr; the subnet split may not exceed 8 bits."
  }
}

variable "name_prefix" {
  type    = string
  default = "tasklab-staging"
}
