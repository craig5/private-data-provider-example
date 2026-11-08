# TODO

- **CI.** A GitHub Actions workflow that runs `make test` and `make compare` on
  every push, so the repo shows it works. Needs Go, Python 3 and OpenTofu on
  the runner.
- **Test the provider glue.** `internal/model` is unit tested, but nothing
  tests `internal/provider`: the schema, reading `data_dir` and
  `email_domain`, and mapping the result into state. A real acceptance test
  needs `terraform-plugin-testing` and an OpenTofu or Terraform binary in CI.
- **Validate names.** Usernames and role names can contain anything,
  including `/`, which the binding keys (`username/role`) use as a separator:
  user `a/b` with role `c` and user `a` with role `b/c` would collide. Decide
  on a charset (say `[a-z0-9_-]`) and enforce it in Go, Python and the HCL.
