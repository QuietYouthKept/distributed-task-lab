locals {
  enabled = var.enable_resources && var.budget_approved
  tags = {
    Project      = "distributed-task-lab"
    Environment  = "staging-sim"
    RunID        = var.run_id
    ResourceTTL  = var.resource_ttl
    Owner        = "tasklab-cloud-acceptance"
    CostApproved = tostring(var.budget_approved)
  }
}

resource "alicloud_vpc" "staging" {
  count      = local.enabled ? 1 : 0
  vpc_name   = "${var.name_prefix}-${var.run_id}"
  cidr_block = var.vpc_cidr
  tags       = local.tags
}

resource "alicloud_vswitch" "staging" {
  count        = local.enabled ? 1 : 0
  vpc_id       = alicloud_vpc.staging[0].id
  zone_id      = var.zone_id
  cidr_block   = var.vswitch_cidr
  vswitch_name = "${var.name_prefix}-${var.run_id}-a"
  tags         = local.tags
}

resource "alicloud_security_group" "staging" {
  count               = local.enabled ? 1 : 0
  security_group_name = "${var.name_prefix}-${var.run_id}"
  vpc_id              = alicloud_vpc.staging[0].id
  security_group_type = "normal"
  description         = "Run-scoped TaskLab staging simulation; no public API or database ingress."
  tags                = local.tags
}

resource "alicloud_security_group_rule" "ssh" {
  count             = local.enabled && var.ssh_cidr != "" ? 1 : 0
  type              = "ingress"
  ip_protocol       = "tcp"
  port_range        = "22/22"
  security_group_id = alicloud_security_group.staging[0].id
  cidr_ip           = var.ssh_cidr
  nic_type          = "internet"
  policy            = "accept"
  priority          = 10
  description       = "SSH from the operator's explicitly approved CIDR only."
}

resource "alicloud_security_group_rule" "postgres_private" {
  count                    = local.enabled ? 1 : 0
  type                     = "ingress"
  ip_protocol              = "tcp"
  port_range               = "5432/5432"
  security_group_id        = alicloud_security_group.staging[0].id
  source_security_group_id = alicloud_security_group.staging[0].id
  nic_type                 = "intranet"
  policy                   = "accept"
  priority                 = 20
  description              = "PostgreSQL only between run-owned ECS instances."
}

resource "alicloud_security_group_rule" "gateway_private" {
  count                    = local.enabled ? 1 : 0
  type                     = "ingress"
  ip_protocol              = "tcp"
  port_range               = "18081/18081"
  security_group_id        = alicloud_security_group.staging[0].id
  source_security_group_id = alicloud_security_group.staging[0].id
  nic_type                 = "intranet"
  policy                   = "accept"
  priority                 = 21
  description              = "Gateway simulator traffic only from run-owned ECS instances."
}

resource "alicloud_instance" "staging" {
  count                      = local.enabled ? (var.create_worker_b ? 2 : 1) : 0
  instance_name              = "${var.name_prefix}-${var.run_id}-${count.index == 0 ? "core" : "worker-b"}"
  host_name                  = "tasklab-${count.index == 0 ? "core" : "worker-b"}"
  instance_type              = count.index == 0 ? var.instance_type : var.worker_b_instance_type
  image_id                   = var.image_id
  system_disk_category       = "cloud_essd"
  system_disk_size           = var.system_disk_size_gib
  vswitch_id                 = alicloud_vswitch.staging[0].id
  security_groups            = [alicloud_security_group.staging[0].id]
  key_name                   = var.ssh_key_name
  instance_charge_type       = "PostPaid"
  internet_max_bandwidth_out = var.ssh_cidr == "" ? 0 : 1
  user_data                  = null
  tags                       = merge(local.tags, { Role = count.index == 0 ? "core" : "worker" })
}
