# Mango

Configuration **man**agement, in **go**... get it? Well, I tried.

Anyway, this repo is me building a toy configuration management engine in go, mostly for learning purposes (primarily testing in go, honestly).

## Configuration Management

`Mango` is intended to be run as a daemon on the system that it will be managing.
`Mango` requires privileges to touch and manage the things you ask it to do (whether via your user when launching the service or a service manager like `Systemd`, that's up to you).
`Mango` is best used with an inventory controlled via git, for configuration as code.

### Inventory
`Mango`'s inventory is based on [aviary.sh's](https://github.com/frameable/aviary.sh) inventory.
Initially, `mango` will be an aviary.sh-compatible daemon, with configurations written as Bash scripts.
As the project develops, the goal is to eventually allow for defining configurations in a yaml based idempotent DSL.

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

## Goals

List of features that I plan to at least try and implement:
- minimum of the following managers:
    - `file`
        - an `enforce` option that will watch the file via inotify or similar and forcefully correct file on demand
        - data sources:
            - go templated files
            - http request data
        - different serializer suppoprt (yml, toml, ini, etc)
    - `systemd`
        - start/stop
        - enable/disable
        - mask/unmask
    - `command`
        - provide ability to run abstract shell commands
        - provide support for an additional `check` type command to validate success/failure for idempotency
- a way to specify ordering/dependencies (ie, write this unit file before attempting to start the service it creates)
- a way to include/override/merge configs
- high concurrency (yay goroutines!)
- native prometheus metrics
