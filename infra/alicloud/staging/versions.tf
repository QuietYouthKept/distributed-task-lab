terraform {
  required_version = ">= 1.9.0, < 2.0.0"

  required_providers {
    alicloud = {
      source  = "aliyun/alicloud"
      version = "= 1.274.0"
    }
  }
}

provider "alicloud" {
  region = var.region
  # Credentials must come from the Alibaba Cloud CLI profile or supported
  # environment variables. Never put AccessKey material in this directory.
}
