# Mango

[![license](https://img.shields.io/github/license/tjhop/mango)](https://github.com/tjhop/mango/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/tjhop/mango)](https://goreportcard.com/report/github.com/tjhop/mango)
[![golangci-lint](https://github.com/tjhop/mango/actions/workflows/golangci-lint.yaml/badge.svg)](https://github.com/tjhop/mango/actions/workflows/golangci-lint.yaml)
[![Latest Release](https://img.shields.io/github/v/release/tjhop/mango)](https://github.com/tjhop/mango/releases/latest)
![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/tjhop/mango/total)

Configuration **man**agement, in **go**... get it? Well, I tried.

Anyway, this repo is me building a toy configuration management engine in go, mostly for learning purposes.

Project/design goals:
- Compatibility with [aviary.sh's inventory format](https://github.com/frameable/aviary.sh)
- Each system manages itself via idempotent scripts/programs
- The daemon should be easy to analyze/inspect
    - [Pprof](https://github.com/google/pprof) enabled
    - Native [Prometheus](https://prometheus.io) metrics
    - Structured logging (`LogFmt` and `JSON`)
    - Tracing for script manager execution stats (Planned)
    - Exemplars to link metrics/logs to specific manager script run traces (Planned)
    - Grafana dashboard (Planned)

## Setup

Download a release appropriate for your system from the [Releases](https://github.com/tjhop/mango/releases) page.
While packages are built for several systems, there are currently no plans to attempt to submit packages to upstream package repositories.

## Usage

### Binary Usage

```bash
mango --inventory.path /path/to/inventory
```

All options:

```bash
~/go/src/github.com/tjhop/mango (main [  ]) -> ./mango -h
 _ __ ___    __ _  _ __    __ _   ___
| '_ ` _ \  / _` || '_ \  / _` | / _ \
| | | | | || (_| || | | || (_| || (_) |
|_| |_| |_| \__,_||_| |_| \__, | \___/
                          |___/

Usage of ./mango:
  -h, --help                                       Prints help and usage information
      --hostname string                            (Requires root) Custom hostname to use [default is system hostname]
  -i, --inventory.path string                      Path to mango configuration inventory
      --inventory.reload-interval string           Time duration for how frequently mango will auto reload and apply the inventory [default disabled]
  -l, --logging.level string                       Logging level may be one of: [trace, debug, info, warning, error, fatal and panic]
      --logging.output string                      Logging format may be one of: [logfmt, json] (default "logfmt")
      --manager.skip-apply-on-test-success apply   If enabled, this will allow mango to skip running the module's idempotent apply script if the `test` script passes without issues
  -v, --version                                    Prints version and build info

Mango is charityware, in honor of Bram Moolenaar and out of respect for Vim. You can use and copy it as much as you like, but you are encouraged to make a donation for needy children in Uganda.  Please visit the ICCF web site, available at these URLs:

https://iccf-holland.org/
https://www.vim.org/iccf/
https://www.iccf.nl/
```

### Container Usage

Since `mango` is intended to be run on the system it is managing and thus requires access to the host system, if you must run `mango` as a container, you may want to use the `--privileged` flag.

```
# docker works too, but podman is wonderful
podman run \
-v /path/to/inventory:/opt/mango/inventory \
--privileged \
ghcr.io/tjhop/mango
```

## Configuration Management

`Mango` is intended to be run as a daemon on the system that it will be managing.
`Mango` requires privileges to touch and manage the things you ask it to do (whether via your user when launching the service or a service manager like `Systemd`, that's up to you).
`Mango` is best used with an inventory controlled via git, for configuration as code.

### Inventory
`Mango`'s inventory is based on [aviary.sh's](https://github.com/frameable/aviary.sh) inventory.
Initially, `mango` will be an aviary.sh-compatible daemon, with configurations written as scripts/executables.

#### Inventory Setup
Please see [aviary.sh's documentation on inventory setup](https://github.com/frameable/aviary.sh#inventory-setup) for more information.

```
mkdir inventory
cd inventory
mkdir {groups,hosts,modules,roles,directives}
touch {groups,hosts,modules,roles,directives}/.gitkeep
git init
git add .
git commit -m "initial commit"
```

*NOTE*: While `aviary.sh`'s inventory system is designed to work with bash
scripts, it's possible to write a module in any language. Mango treats module
test scripts as optional (yet recommended), and module/host variables are
similarly optional. The module's `apply` script is the only required script for
a module, so it's possible to simply use the `apply` script as a launcher to
whatever other idempotent scripts/configs written in other languages.

#### Differences from [Aviary.sh](https://github.com/frameable/aviary.sh)

| Aviary.sh | Mango |
| --- | --- |
| Short lived process | Long lived process |
| Runs as scheduled service (`cron`, `systemd-timer`, etc) | Runs as systemd daemon (Systemd unit, etc) |
| Inventory updated during config management runs | Inventory updated via scheduled service (`cron`, `systemd-timer`, etc) |
| Inventory scripts executed directly by shell | Inventory scripts executed by [Go shell interpreter library](https://github.com/mvdan/sh). Bash is supported [with some caveats](https://github.com/mvdan/sh#caveats) |

#### Inventory files per component and what they do

| Inventory Component | File/Directory Name | File Type | Description | Required | Allows templating |
| --- | --- | --- | --- | --- | --- |
| `directives` | _any allowed_ | Bash script | "one-off" commands that get run only a single time and only if the file has been modified within the last 24 hours | No | Yes |
| `modules` | `apply` | Bash script | idempotent bash script to get the system to the desired state | Yes | Yes |
| `modules` | `test` | Bash script | test script to validate if system is in the desired state | No | Yes |
| `modules` | `variables` | Bash script | script containing variables to set for the module's execution context for `apply` and `test` scripts | No | Yes |
| `modules` | `requires` | Newline delimited list | List of other modules that are required to apply before this module can apply (dependency ordering) | No | No |
| `modules` | `templates/` | Directory | Contains Go text templates as `.tpl` files  | No | No |
| `roles` | `modules` | Newline delimited list | List of modules that are included in/executed as part of this role | No | No |
| `hosts` | `modules` | Newline delimited list | List of modules that are included in/executed as part of the defined host | No | No |
| `hosts` | `roles` | Newline delimited list | List of roles that are included in/executed as part of the defined host | No | No |
| `hosts` | `variables` | Bash script | script containing variables to set for the host's execution context for `apply` and `test` scripts | No | Yes |
| `groups` | `glob` | Newline delimited list | List of glob patterns that are members of this group. Glob patterns are matched against the hostname of the system | No | No |
| `groups` | `regex` | Newline delimited list | List of regular expression patterns that are members of this group. Regular expression patterns are matched against the hostname of the system | No | No |
| `groups` | `roles` | Newline delimited list | List of roles assigned to members of this group | No | No |
| `groups` | `modules` | Newline delimited list | List of modules assigned to members of this group | No | No |
| `groups` | `variables` | Bash script | script containing variables to set for the host's execution context for `apply` and `test` scripts | No | Yes |

## Monitoring and Alerting

### Metrics

Mango exposes [Prometheus metrics](https://prometheus.io/) on port `9555` on all interfaces by default.

### Alerts

Sample alerts are planned.

### Dashboards

A Grafana dashboard is in progress.

## Development

### Required Software

This project uses:
- [goreleaser](https://goreleaser.com/) to manage builds.
- [podman compose](https://github.com/containers/podman-compose) to run supplementary services during testing
- [podman](https://podman.io/) to build and run containers
- [aardvark-dns](https://github.com/containers/aardvark-dns) for container DNS resolution (default in podman 4+, required for compose service name resolution within containers.

### Build From Source

Most development work can be done with the included Makefile:

```bash
~/go/src/github.com/tjhop/mango (main [ ]) -> make help
# autogenerate help messages for comment lines with 2 `#`
 help:                  print this help message
 tidy:                  tidy modules
 fmt:                   apply go code style formatter
 lint:                  run linters
 binary:                build a binary
 build:                 alias for `binary`
 container:             build container image with binary
 image:                 alias for `container`
 podman:                alias for `container`
 docker:                alias for `container`
 test-container:        build test containers with binary for testing purposes
 test-image:            alias for `container`
 test-podman:           alias for `container`
 test-docker:           alias for `container`
 services:              use podman compose to spin up local grafana, prometheus, etc
 run-test-containers    use podman compose to spin up test containers running systemd for use with the test inventory
 reload-test-inventory: use podman to reload the mango systemd service running in the ubuntu test container
 stop:                  stop test environment and any other cleanup
 clean:                 alias for `stop`
```

### Testing

#### Inventory Testing
A [skeleton inventory ](./test/mockup/inventory/) is included for use with testing:

```bash
make run-test-inventory
```

#### Code Testing

Doesn't exist yet :')

### Contributions
Commits *must* follow [Conventional Commit format](https://www.conventionalcommits.org/en/v1.0.0/). This repository uses [GoReleaser](https://goreleaser.com/) and semver git tags that are determined by the type of commit.
