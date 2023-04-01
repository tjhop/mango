# Mango Test Inventory

Small inventory for use with local testing and verifying functionality. Not necessarily intended to be used as a test suite, but by nature of testing functionalities, it may become one. As such, scripts in this inventory should avoid making permanent changes to the underlying system.

## Usage

### Direct (Binary)

```bash
# from mango project root

# build binary locally
goreleaser build --clean --single-target --snapshot

# run binary (with privileges, since it needs to create log dirs and launch privileged processes)
# NOTE: explicitly set alternate hostname to match the host in this inventory
sudo ./dist/mango_linux_amd64_v1/mango --inventory.path ./test/mockup/inventory --logging.level debug --hostname testbox
```
