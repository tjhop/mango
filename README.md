# Mango

Configuration **man**agement, in **go**... get it? Well, I tried.

Anyway, this repo is me building a toy configuration management engine in go, mostly for learning purposes (primarily testing in go, honestly).

## Configuration Management

`Mango` is intended to be run as a daemon on the system that it will be managing.
`Mango` requires privileges to touch and manage the things you ask it to do (whether via your user when launching the service or a service manager like `Systemd`, that's up to you).
`Mango` is best used with an `actions.yml` file controlled via git, for configuration as code.

### Managers

Configuration management in `mango` is based around the concept of `managers`.
`Managers` will be idempotent, and take only the actions needed get the system in the desired state.
There will be different types of `managers`, based on a `manager` interface.

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
