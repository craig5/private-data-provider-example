# private-data-provider-example

Example of a "data provider" for OpenTofu Day talk 2026: companion code for
*Private Data Providers for Clean Data Parsing: Clean HCL Through Go*.

The same YAML input is turned into the same users, roles and user-to-role
bindings twice:

- [`tf-hcl-only/`](tf-hcl-only/main.tf) parses the YAML in HCL with `yamldecode`,
  `flatten`, `try` and `for` expressions.
- [`tf-with-provider/`](tf-with-provider/main.tf) asks a small private provider for
  already-parsed, typed maps and just uses them in `for_each`.

`terraform_data` stands in for real identity resources (a user, a group, a
membership), so everything runs locally with no credentials.

## Layout

| Path | What |
| --- | --- |
| `data/roles.yaml` | Every role a user can hold |
| `data/teams/*.yaml` | One file per team, listing its users |
| `tf-hcl-only/` | Parsing in HCL |
| `tf-hcl-only/tests/` | Python checks on the data: a second copy of the HCL's rules |
| `tf-with-provider/` | Parsing in the provider |
| `provider/internal/model/` | All the parsing rules, plain Go, unit tested |
| `provider/internal/provider/` | Thin OpenTofu glue: schema plus one call to `model.Load` |
| `provider/cmd/c5egprov-show/` | A CLI that prints the parsed data, using the same `model.Load` |

## The rules

Both versions turn valid data into the same result, following these rules.
On invalid data the provider rejects more, and says where; see
[Where they differ](#where-they-differ).

- there is at least one team file, `*.yaml` or `*.yml`
- every team file names its `team`
- every user has a `username` and a `full_name`, and usernames are unique
  across teams
- a user with no `roles` key gets the team's `default_roles`; a `roles` key
  that is there but empty is an error
- a user lists each role at most once
- every role a user holds must exist in `roles.yaml`
- `disabled: true` users are skipped
- `email` defaults to `<username>@<email_domain>`; an empty `email` is an
  error

## Running it

Needs Go, Python 3, and either OpenTofu or Terraform.

```bash
make tf-hcl-only            # plan the HCL-only version
make tf-with-provider       # install the provider and plan the provider version
make show                   # print the parsed data as tables; FORMAT=json for JSON
make compare                # check both produce the same bindings
make test                   # both test suites: Python for tf-hcl-only/, Go for the provider
```

The Python tests run from a virtualenv in `.venv/`, created on first use from
the pinned `tf-hcl-only/tests/requirements.txt`. To change a dependency, edit
`requirements.in` and run `make requirements`, which needs
[uv](https://docs.astral.sh/uv/).

## Seeing the parsed data

`c5egprov-show` uses the same parsing code as the provider and prints the
result. It takes just the data directory, like `tf-hcl-only/main.tf` does. No
OpenTofu, no plan:

```bash
make show               # users, roles and bindings as tables
make show FORMAT=json   # exactly what the data source hands to OpenTofu
```

The JSON uses the provider's attribute names, so it matches
`data.c5egprov_directory.this` field for field. A bad file fails with the
same `file: line N` error a plan would give.

Because it is an ordinary Go program, it also runs under a debugger, which a
plan never can:

```bash
cd provider
dlv debug ./cmd/c5egprov-show -- -data-dir ../data
```

## How the provider is installed

`make tf-with-provider` builds the provider straight into

```
~/.terraform.d/plugins/providers.example.internal/craig5/c5egprov/0.1.0/<os>_<arch>/
```

OpenTofu and Terraform both look there before trying a registry, so a plain
`init` finds it: no CLI config, nothing published. `make uninstall` removes it.

- **The hostname is not a real registry.** With a bare `craig5/c5egprov`,
  OpenTofu and Terraform each assume their own public registry, and a machine
  without the local copy could download someone else's provider of the same
  name. `providers.example.internal` cannot resolve, which rules that out.
- **The lock file records the provider's checksum.** Rebuild the same version
  with different code and `init` and `plan` both reject it - and
  `init -upgrade` does not help, because the version has not changed. Delete
  `.terraform.lock.hcl` and run `init` again; the Makefile does this after
  every rebuild. Outside a demo, bump the version.
- **Your CLI config must not have a `provider_installation` block.** An
  explicit one replaces the directory lookup above.

While working on the provider itself, `dev_overrides` in a CLI config is the
faster loop: it skips `init`, versions and the lock file, and uses the binary
straight from wherever you build it.

## Naming the provider

`c5egprov` is squashed on purpose:

- **No `_`.** It separates the provider name from the resource name, as in
  `c5egprov_directory`, so OpenTofu and Terraform both reject it.
- **`-` depends on where the binaries live.** OpenTofu and Terraform accept it
  (`hashicorp/google-beta` has one), but whether you can use it depends on how
  you serve or host the provider binaries: some private registries reject it.
- **`c5` is the org prefix.** The `craig5/` namespace keeps the addresses
  distinct, but resource names carry only the provider name, so two providers
  both called `people` would collide inside one module.

## Where they differ

Both produce the same plan from valid input. The difference is what happens
with input that is not valid:

| Input | `tf-hcl-only/` | `tf-with-provider/` |
| --- | --- | --- |
| Typo `role:` for `roles:` | Silently gets the team's default roles | `platform.yaml: line 7: unknown key "role"` |
| `roles: admin` (a string) | `Invalid function argument` in `main.tf`, not the YAML | ``line 7: cannot unmarshal !!str `admin` into []string`` |
| Unknown role | Caught, but only by a hand-written `precondition` | `file:line: user "x" has role "y", which is not in the roles file` |
| Same user in two teams | `Duplicate object key` with no file name | Both file:line locations |
| `teams/` missing or empty | Plans with no users at all - against a real system, deleting every one | `no team files (*.yaml or *.yml) in .../teams` |
| `email: ""` | Creates the user with an empty email | `user "x" has an empty email` |
| Testing any of this | A separate Python suite that re-implements the rules | `go test`, against the code that does the parsing |

## License

MIT. See [LICENSE](LICENSE).
