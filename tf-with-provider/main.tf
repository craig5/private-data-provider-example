# WITH PROVIDER: the same result as ../tf-hcl-only, with the parsing moved
# into a private provider (../provider). The rules - and their tests - live in
# Go. The HCL only says what to create.
#
# Run it with `make tf-with-provider` from the repo root, which installs the
# provider into ~/.terraform.d/plugins, where OpenTofu and Terraform both look
# for providers before trying a registry.

terraform {
  required_providers {
    # A hostname that is not a real registry, so this can only ever resolve
    # to the locally installed copy - never to a public provider that happens
    # to share the name.
    c5egprov = {
      source  = "providers.example.internal/craig5/c5egprov"
      version = "0.1.0"
    }
  }
}

variable "email_domain" {
  type    = string
  default = "example.com"
}

data "c5egprov_directory" "this" {
  data_dir     = "${path.module}/../data"
  email_domain = var.email_domain
}

# The provider is written with the Plugin SDK v2, which can return lists of
# objects but not maps of them. for_each wants maps, so key each list here.
locals {
  users    = { for u in data.c5egprov_directory.this.users : u.username => u }
  roles    = { for r in data.c5egprov_directory.this.roles : r.name => r }
  bindings = { for b in data.c5egprov_directory.this.bindings : "${b.user}/${b.role}" => b }
}

resource "terraform_data" "user" {
  for_each = local.users

  input = {
    username  = each.value.username
    full_name = each.value.full_name
    email     = each.value.email
    team      = each.value.team
  }
}

resource "terraform_data" "role" {
  for_each = local.roles

  input = {
    name        = each.value.name
    description = each.value.description
  }
}

resource "terraform_data" "binding" {
  for_each = local.bindings

  input = {
    user = terraform_data.user[each.value.user].output.username
    role = terraform_data.role[each.value.role].output.name
  }
}

output "bindings" {
  value = sort(keys(local.bindings))
}
