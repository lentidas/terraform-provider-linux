# Changelog

## [0.8.0](https://github.com/lentidas/terraform-provider-linux/compare/v0.7.1...v0.8.0) (2026-11-25)


### Features

* takes ownership of the provider as lentidas/terraform-provider-linux ([5834adc](https://github.com/lentidas/terraform-provider-linux/commit/5834adcda7f7c45cf2f1faee662a93a083adf288))


### Bug Fixes

* forgets file and directory resources whose remote path is missing ([7ad6e72](https://github.com/lentidas/terraform-provider-linux/commit/7ad6e722ce92fa108ecc080b579b5b8b603d9018))


### Documentation

* adds examples and templates for the generated documentation ([8930540](https://github.com/lentidas/terraform-provider-linux/commit/8930540a898b29ab9f856d48bfb40925a4c464fe))
* describes every attribute of the provider schema ([2fb5cd8](https://github.com/lentidas/terraform-provider-linux/commit/2fb5cd89e5dc95b366c13e32bffaea1816a17971))
* documents local development with mise and the test server ([7278f5d](https://github.com/lentidas/terraform-provider-linux/commit/7278f5de8e6095fc3cbc360f322b9d063ddad3c7))
* documents releases and versioning ([2de1a46](https://github.com/lentidas/terraform-provider-linux/commit/2de1a4650eb10e218fe9c94c576162fbb516b758))
* documents the generated acceptance test key ([08d65cd](https://github.com/lentidas/terraform-provider-linux/commit/08d65cd3faf631ee7011755d04165ad1cd76cf23))
* regenerates the provider documentation ([1be0052](https://github.com/lentidas/terraform-provider-linux/commit/1be00529ce8677c48a088a9b3bbd3b6884e7ff26))


### Styles

* makes some small style changes ([c324e32](https://github.com/lentidas/terraform-provider-linux/commit/c324e324c229f2370e766c6b4fa689ef93d2016f))


### Miscellaneous Chores

* adds a codeowners file and renames the license file ([65ce40a](https://github.com/lentidas/terraform-provider-linux/commit/65ce40aa6925b5a5b7f63f1398e36adf19efdb14))
* adds Renovate configuration ([9edc889](https://github.com/lentidas/terraform-provider-linux/commit/9edc88906d4587ec71b96aa10f7115167b17dcf7))
* lets renovate bump the go directive ([badedb1](https://github.com/lentidas/terraform-provider-linux/commit/badedb14ae88140eff4738eb91e42b2aa50fe509))
* merge pull request [#1](https://github.com/lentidas/terraform-provider-linux/issues/1) from lentidas/feat/take-ownership-of-provider ([5834adc](https://github.com/lentidas/terraform-provider-linux/commit/5834adcda7f7c45cf2f1faee662a93a083adf288))
* merge pull request [#3](https://github.com/lentidas/terraform-provider-linux/issues/3) from lentidas/ci/tests-workflow-and-release-prs ([fc5dfc5](https://github.com/lentidas/terraform-provider-linux/commit/fc5dfc53614856ba6aa39d98ea48f5009994fba2))


### Build System

* adds a docs task backed by a pinned tfplugindocs ([5026fff](https://github.com/lentidas/terraform-provider-linux/commit/5026fffe19f55ade04bb851037783caf08eff2ea))
* adds a local ssh server image for the acceptance tests ([66a6213](https://github.com/lentidas/terraform-provider-linux/commit/66a62136bb973d4b7033c9dd62a2e049896f174f))
* drops linker flags for variables that do not exist ([8273bac](https://github.com/lentidas/terraform-provider-linux/commit/8273bac717d651b6be1b5180e5f8784611500f83))
* generates the acceptance test ssh key instead of committing one ([780dc37](https://github.com/lentidas/terraform-provider-linux/commit/780dc37455fda38697f353c112172f3721af91fc))
* lists golang.org/x/crypto as a direct dependency ([4a72c61](https://github.com/lentidas/terraform-provider-linux/commit/4a72c612bd3fd1d9eb7d43a977040f4b9327eacc))
* reads the signing passphrase from stdin ([d85c694](https://github.com/lentidas/terraform-provider-linux/commit/d85c694e9d6f81b4ee5866f6573cba6ae4373a48))
* replaces the makefile with mise tasks ([54cae4b](https://github.com/lentidas/terraform-provider-linux/commit/54cae4bfb48750e7794a8c298aef91db7c3b5d15))
* runs the acceptance tests with opentofu ([af85577](https://github.com/lentidas/terraform-provider-linux/commit/af8557731959e40bfe61f51dff2f5da513fbdca9))
* takes the go version from go.mod ([d93e73d](https://github.com/lentidas/terraform-provider-linux/commit/d93e73d5bb3e04ea9c9d500b09900d1c62dc423c))
* updates the pinned go toolchain to 1.27.4 ([b5e2ceb](https://github.com/lentidas/terraform-provider-linux/commit/b5e2ceb516fffcf425dd22d6ed14b9ed6d8d8b98))


### Continuous Integration

* adds commit message checks on pull requests ([b542fc7](https://github.com/lentidas/terraform-provider-linux/commit/b542fc7a5cabdaeddb3c9f3abcb8aae98c85e7b0))
* automates releases with release-please and goreleaser ([9e2759c](https://github.com/lentidas/terraform-provider-linux/commit/9e2759c6b3fbdaf1f2dc702cec3c93e44c4e9d76))
* builds and signs a snapshot release on every change ([fdd325b](https://github.com/lentidas/terraform-provider-linux/commit/fdd325b9c0960712b4be1f5889388f13335ed697))
* checks that the generated documentation is up to date ([12130e0](https://github.com/lentidas/terraform-provider-linux/commit/12130e0c6c192b7e6929a5f18d25460ba4de37a5))
* renames the go-tests workflow to tests ([4de6fbe](https://github.com/lentidas/terraform-provider-linux/commit/4de6fbee4e585b76a61563a79ad1c3278e73c79b))
* renames the tests workflow and filters jobs by changed files ([fc5dfc5](https://github.com/lentidas/terraform-provider-linux/commit/fc5dfc53614856ba6aa39d98ea48f5009994fba2))
* runs the tests and the acceptance tests on GitHub Actions ([002cf96](https://github.com/lentidas/terraform-provider-linux/commit/002cf968877374dadec1ce25fa53a0273bbdb5d9))
* runs the workflow jobs only when the files they depend on change ([2713c89](https://github.com/lentidas/terraform-provider-linux/commit/2713c89411fa3cbba7f053de758483fb72dd6c53))
