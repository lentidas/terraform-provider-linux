Terraform Provider Linux
========================

[![Release](https://img.shields.io/github/v/release/lentidas/terraform-provider-linux)](https://github.com/lentidas/terraform-provider-linux/releases) [![Tests](https://img.shields.io/github/actions/workflow/status/lentidas/terraform-provider-linux/tests.yaml?label=tests)](https://github.com/lentidas/terraform-provider-linux/actions/workflows/tests.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/TelkomIndonesia/terraform-provider-linux)](https://goreportcard.com/report/github.com/TelkomIndonesia/terraform-provider-linux)

- Website: <https://registry.terraform.io/namespaces/TelkomIndonesia>

Requirements
------------

- [Terraform](https://www.terraform.io/downloads.html) 0.12.x
- [Go](https://golang.org/doc/install) at the version of the `go` directive in `go.mod`, which mise installs (to build the provider plugin)

Usage
-----

```hcl
provider "linux" {
    host = "127.0.0.1"
    port = 22
    user = "root"
    password = "root"
}

resource "linux_directory" "directory" {
    path = "/tmp/linux/dir"
    owner = 1000
    group = 1000
    mode = "755"
    overwrite = true
    recycle_path = "/tmp/recycle"
}

resource "linux_file" "file" {
    path = "/tmp/linux/file"
    content = <<-EOF
        hello world
    EOF
    owner = 1000
    group = 1000
    mode = "644"
    overwrite = true
    recycle_path = "/tmp/recycle"
}


locals {
    package_name = "apache2"
}

resource "linux_script" "install_package" {
    lifecycle_commands {
        create = "apt update && apt install -y $PACKAGE_NAME=$PACKAGE_VERSION"
        read = "apt-cache policy $PACKAGE_NAME | grep 'Installed:' | grep -v '(none)' | awk '{ print $2 }' | xargs | tr -d '\n'"
        update = "apt update && apt install -y $PACKAGE_NAME=$PACKAGE_VERSION"
        delete = "apt remove -y $PACKAGE_NAME"
    }
    environment = {
        PACKAGE_NAME = local.package_name
        PACKAGE_VERSION = "2.4.18-2ubuntu3.4"
    }
    triggers = {
        PACKAGE_NAME = local.package_name"
    }
}
```

Developing The Provider
-----------------------

### Local OpenTofu development

This fork uses the OpenTofu provider address `registry.opentofu.org/lentidas/linux`. The current local package version is `0.7.2`; it is not a published registry release. The Go module path remains unchanged to keep the patch compatible with upstream imports.

Development tasks are defined in `mise.toml`, which also pins the Go version used locally and in CI. Install [mise](https://mise.jdx.dev/) and run `mise tasks` to list them. The first `mise run` asks you to trust the file (`mise trust`).

Build the provider into a filesystem mirror on your current platform:

```sh
mise run build-local
mise run test
```

For the Linux Management caller, set `LINUX_PROVIDER_REPO` to this checkout's absolute path, then run the following from the caller directory. Create the ignored mirror symlink once; its `provider-local.tfrc` restricts installation of this provider to that mirror, while other providers retain their normal installation method.

```sh
export LINUX_PROVIDER_REPO="/absolute/path/to/terraform-provider-linux"
mkdir -p .terraform
ln -s "$LINUX_PROVIDER_REPO/build/providers" .terraform/provider-mirror
TF_CLI_CONFIG_FILE="$PWD/provider-local.tfrc" tofu init
tofu validate
tofu plan
```

The caller pins `lentidas/linux` to `0.7.2`; the reusable module accepts `>= 0.7.2`. No global OpenTofu configuration changes are needed. Once initialized, normal `tofu` commands use the installed fork.

When migrating an existing encrypted local state, run this command separately before `init`, with no concurrent state operations. OpenTofu locks the state and creates a backup. The existing variable file supplies the encryption passphrase; do not put its value on the command line.

```sh
tofu state replace-provider -auto-approve -var-file=terraform.tfvars \
  registry.opentofu.org/telkomindonesia/linux registry.opentofu.org/lentidas/linux
```

> [!IMPORTANT]
> Treat mirror package versions as immutable. For a subsequent patch, build with a new version (`mise run build-local <version>`) and bump the caller's version pin before initializing. Do not overwrite an installed package or disable lock-file checksum verification. A mirror-built lock entry only covers the platform on which it was generated.

The file and directory resources now forget genuinely missing paths during refresh so the next plan proposes recreation. Permission, command and SSH failures remain errors and retain the resource ID. Regression tests use a loopback-only SSH server that replies to commands without executing them; they do not require a Linux VM.

The available tasks are:

| Task | What it does |
| --- | --- |
| `mise run build` | Builds the provider binary into the current directory |
| `mise run build-local [version]` | Builds into the local OpenTofu mirror described above (default version `0.7.2`) |
| `mise run vet` | Runs `go vet` |
| `mise run test` | Runs the unit tests, including the nested SSH communicator module; extra `go test` arguments go after `--`, for example `mise run test -- -race -run TestLinuxFileRead` |
| `mise run testacc` | Starts the SSH server for the acceptance tests if needed, then runs them with OpenTofu |
| `mise run ssh-key` | Generates the throwaway SSH key pair of the acceptance tests unless it exists (also done by `ssh-up`) |
| `mise run ssh-up` / `mise run ssh-down` | Starts or removes the SSH server on its own |
| `mise run docs` | Regenerates `docs/` from the schema, templates and examples, then validates it |

The acceptance tests need an SSH server that root can log in to. [`build/docker`](build/docker) defines a disposable one (Debian with OpenSSH, built locally, no third-party image), authorizing a throwaway ed25519 key and the password `root`. `mise run ssh-key` generates the key pair into the git-ignored `build/ssh` directory when it does not exist, so no private key is committed; the tests read it from there, or from `TEST_ACC_LINUX_PROVIDER_PRIVATE_KEY_FILE`, and fall back to the password without it. It listens on `127.0.0.1:2222` only, and `testacc` points the tests there through `TEST_ACC_LINUX_PROVIDER_HOST` and `TEST_ACC_LINUX_PROVIDER_PORT` (see `mise.toml`). Docker with Compose is required, and the OpenTofu CLI the tests drive is installed by mise at the version pinned in `mise.toml`. To run the tests against another machine, set the `TEST_ACC_LINUX_PROVIDER_*` variables [read by the test code](linux/linux_test.go) yourself and call `go test` directly with `TF_ACC=1`.

> [!WARNING]
> Never point the acceptance tests at a machine that matters: they create, change and delete files as root.

### Documentation

The pages in `docs/` are generated by [tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs) and must not be edited by hand. The reference sections come from the `Description` of every resource, data source and attribute in the Go schema, so a behavior change and its documentation belong in the same commit. The prose and examples around them live in `templates/` and `examples/`, in the paths tfplugindocs expects (see its [conventional paths](https://github.com/hashicorp/terraform-plugin-docs#conventional-paths)). Run `mise run docs` after changing any of them and commit the result. The `docs` workflow runs the same task and fails when the committed pages differ from the generated ones.

The OpenTofu registry reads the documentation from the repository at the release tag, preferring `docs/` in this layout, so the pages have to be committed before a release is tagged. A published version is immutable, so a fix to its documentation needs a new release. The `docs` task installs Terraform at the version pinned in `mise.toml`, because tfplugindocs only knows how to drive a Terraform CLI. The generator itself is pinned in `tools/go.mod`, a separate module, so that its dependencies do not change those of the provider.

### Continuous integration

The `tests`, `docs` and `goreleaser-check` workflows run on pull requests and on pushes to `main`, but their jobs only run when a file they depend on changed. The mapping lives in `.github/path-filters.yaml` and is evaluated by the reusable `changes` workflow. The filtering is done with job-level conditions instead of `on.<event>.paths`, because a workflow that is not triggered never reports its checks, which would leave the required checks of the `main` ruleset pending forever, whereas a skipped job counts as passed. Add the path to the filter file when a workflow starts depending on a new file. The `commits-checks` workflow always runs, and a release pull request, which only touches the changelog and the version manifest, skips everything else.

### Releases and versioning

Versions follow [Semantic Versioning](https://semver.org/) and are derived by [release-please](https://github.com/googleapis/release-please) from the [Conventional Commits](https://www.conventionalcommits.org/) merged into `main` (for example `fix: ...` or `feat: ...`), which the `commits-checks` workflow verifies on pull requests. Release-please keeps a `chore: release <version>` pull request open with the changelog. Merging it creates the `v<version>` tag and GitHub release, and the tag triggers GoReleaser to build, sign and attach the provider archives. While the major version is 0, a breaking change bumps the minor version and a feature bumps the patch version. The version history before the fork is excluded through `bootstrap-sha` in `.release-please-config.json`, which can be removed once the first release pull request is merged.

The workflows read their credentials from repository secrets (for example `REPOSITORY_BOT_APP_ID` for the GitHub App and `GPG_PRIVATE_KEY` for signing); see `.github/workflows/` for the complete list. A GitHub App token is used instead of `GITHUB_TOKEN` because tags created with the latter do not trigger the release workflow. Dependency updates come from Renovate, configured in `.renovaterc.json`.

> [!IMPORTANT]
> A GitHub release is not a registry listing. Publishing `lentidas/linux` to the OpenTofu registry is a separate step, and the first automated release is expected to also be `0.7.2`, the version used by the local mirror build above. Switch from the mirror to the released package with `tofu init -upgrade` and review the regenerated lock file.
