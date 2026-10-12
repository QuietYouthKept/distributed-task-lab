output "vpc_id" {
  value       = try(alicloud_vpc.staging[0].id, null)
  description = "Run-owned VPC ID; null while both creation gates are false."
}

output "vswitch_id" {
  value       = try(alicloud_vswitch.staging[0].id, null)
  description = "Run-owned vSwitch ID; null while both creation gates are false."
}

output "security_group_id" {
  value       = try(alicloud_security_group.staging[0].id, null)
  description = "Run-owned Security Group ID; null while both creation gates are false."
}

output "ecs_instance_ids" {
  value       = [for instance in alicloud_instance.staging : instance.id]
  description = "Run-owned ECS IDs; empty until resources and budget are both approved."
}

output "ecs_private_ips" {
  value       = [for instance in alicloud_instance.staging : instance.private_ip]
  description = "Private addresses for SSH tunnels and Worker-to-core traffic."
}

output "ecs_public_ips" {
  value       = [for instance in alicloud_instance.staging : instance.public_ip]
  description = "May be empty when ssh_cidr is empty and public bandwidth is disabled."
}
