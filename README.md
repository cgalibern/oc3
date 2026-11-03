[![Go](https://github.com/opensvc/oc3/actions/workflows/go.yml/badge.svg)](https://github.com/opensvc/oc3/actions/workflows/go.yml)

## configuration example
```yaml
db:
  password: xxxxxxx
  log:
    level: info
    slow_query_threshold: 500ms

feeder:
  addr: 127.0.0.1:8080
  pprof:
    ux.enable: true
    net.enable: true
  metrics:
    enable: true
  ui:
    enable: true

server:
  tx: true
  addr: 127.0.0.1:8081
  auth:
    # Users signing in with their collector password (HTTP Basic). Set to false
    # once everyone signs in through OpenID Connect (server.oidc), which it then
    # requires: passwords are no longer accepted nor set. Nodes keep their own
    # authentication.
    basic_users: true
  pprof:
    ux.enable: true
    net.enable: true
  metrics:
    enable: true
  ui:
    enable: true

scheduler:
  task:
    purge_services:
      # Delete, with their history, the services that no instance reports
      # anymore and whose status is older than max_age. Services tagged
      # @purge are deleted as soon as they have no instance left, whatever
      # max_age. 0 (the default) disables the age based purge. Units: s, m,
      # h, d. At most batch_size services are deleted per hourly run.
      max_age: 0
      batch_size: 1000
  pprof:
    ux.socket: /oc3/scheduler.pprof
    ux.enable: true
    net.enable: true
  metrics:
    enable: true

messenger:
  url: http://0.0.0.0:8889
  key: magix123
  pprof:
    ux:
      enable: true
    net:
      enable: true
  metrics:
    enable: true

worker.slow:
  addr: 127.0.0.1:8100
  tx: true
  pprof:
    ux.enable: true
    net.enable: true
  runners: 3
  metrics:
    enable: true
  queues:
    - "daemon_status"
    - "daemon_ping"

worker.fast:
  addr: 127.0.0.1:8101
  tx: true
  pprof:
    ux.enable: true
    net.enable: true
  runners: 3
  metrics:
    enable: true
  queues:
    - "instance_resource_info"
    - "instance_status"
    - "instance_action"
    - "node_checks"
    - "node_disk"
    - "object_config"
    - "san_switch"
    - "system"
```

## manual build docker image

    docker build \
        --build-arg BUILDTIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --build-arg VERSION="$(git describe --tags --abbrev)" \
        -t opensvc/oc3 .