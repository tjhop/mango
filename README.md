# Mango

Configuration **man**agement, in **go**... get it? Well, I tried.

Anyway, this repo is me building a toy configuration management engine in go, mostly for learning purposes.

Project/design goals:
- Compatibility with [aviary.sh's inventory format](https://github.com/frameable/aviary.sh)
- Each system manages itself via idempotent scripts/programs
- The daemon should be easy to analyze/inspect
    - [Pprof](https://github.com/google/pprof) enabled
    - Native [Prometheus](https://prometheus.io) metrics
    - Structured logging
    - Grafana dashboard (Planned)

## Setup

Download a release appropriate for your system from the [Releases](https://github.com/tjhop/mango/releases) page.
While packages are built for several systems, there are currently no plans to attempt to submit packages to upstream package repositories.

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
mkdir {hosts,modules,roles,directives}
touch {hosts,modules,roles,directives}/.gitkeep
git init
git add .
git commit -m "initial commit"
```

## Development

### Build From Source

This project uses [goreleaser](https://goreleaser.com/) to manage builds.
To manually make a build for local development/testing, you can clone this project and run:

```bash
goreleaser build --rm-dist --single-target --snapshot
```

### Testing

A [skeleton inventory ](./test/mockup/inventory/) is included for use with testing:

```bash
goreleaser build --rm-dist --single-target --snapshot
./dist/mango_linux_amd64_v1/mango --inventory.path ./test/mockup/inventory/ --logging.level debug
```

### Contributions
Commits *must* follow [Conventional Commit format](https://www.conventionalcommits.org/en/v1.0.0/). This repository uses [GoReleaser](https://goreleaser.com/) and semver git tags that are determined by the type of commit.
