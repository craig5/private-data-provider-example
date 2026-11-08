# HCL ONLY: parse the YAML directly in HCL.
#
# This works, and it implements the same rules as the provider used in
# ../tf-with-provider. But every rule is an expression to read, not code to
# test:
#
#   - a user with no roles key gets the team's default_roles
#   - usernames are unique, and a user lists each role at most once
#   - every role a user holds must exist in roles.yaml
#   - disabled users are skipped
#   - email defaults to <username>@<email_domain>
#
# It does not catch everything the provider does: see "Where they differ" in
# the README.
#
# terraform_data stands in for real identity resources (a user, a group, a
# group membership) so this runs anywhere with no credentials.

variable "email_domain" {
  type    = string
  default = "example.com"
}

locals {
  data_dir = "${path.module}/../data"

  # A role with no body decodes as null, hence the try().
  roles = {
    for name, r in yamldecode(file("${local.data_dir}/roles.yaml")).roles :
    name => {
      name        = name
      description = try(r.description, "")
    }
  }

  teams = [
    for f in sort(fileset("${local.data_dir}/teams", "*.{yaml,yml}")) :
    yamldecode(file("${local.data_dir}/teams/${f}"))
  ]

  # Teams -> users. The inner for builds a list per team; flatten joins them so
  # the outer for can key every user by username. try() falls back to the team's
  # default_roles, then to an empty list that a precondition rejects.
  users = {
    for u in flatten([
      for t in local.teams : [
        for u in t.users : {
          username  = u.username
          full_name = u.full_name
          team      = t.team
          email     = try(u.email, "${u.username}@${var.email_domain}")
          roles     = tolist(try(u.roles, t.default_roles, []))
        } if !try(u.disabled, false)
      ]
    ]) : u.username => u
  }

  # Users x roles -> one binding each. Same shape again: build nested lists,
  # flatten, then key.
  bindings = {
    for b in flatten([
      for u in local.users : [
        for r in u.roles : { user = u.username, role = r }
      ]
    ]) : "${b.user}/${b.role}" => b
  }
}

resource "terraform_data" "user" {
  for_each = local.users

  input = {
    username  = each.value.username
    full_name = each.value.full_name
    email     = each.value.email
    team      = each.value.team
  }

  lifecycle {
    precondition {
      condition     = length(each.value.roles) > 0
      error_message = "User ${each.key} has no roles, and team ${each.value.team} has no default_roles."
    }
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

  lifecycle {
    precondition {
      condition     = contains(keys(local.roles), each.value.role)
      error_message = "User ${each.value.user} has role ${each.value.role}, which is not in roles.yaml."
    }
  }
}

output "bindings" {
  value = sort(keys(local.bindings))
}
