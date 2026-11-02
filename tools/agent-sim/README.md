# agent-sim

Load test of the oc3 feed: simulated OpenSVC v3 agents sending to the feeder
what real agents send, for as many nodes and services as asked.

## What an agent sends

As the om3 daemon does (`daemon/collector`, `core/object/node_push_*`), with the
payload shapes and sizes of a real v3 agent:

| Sender | Request | When |
|---|---|---|
| the speaker of each cluster (its first node) | `POST /daemon/status`: the whole cluster dataset and the changes | at most every `--status-delay` (10s), when objects changed |
| | `POST /daemon/ping` | every `--ping-interval` (60s) without changes; a 204 makes it send the whole status again |
| | `POST` then `PUT /instance/action` | begin and end of each action, `--actions-per-day` a service |
| | `POST /instance/resource_info` | every `--info-interval` (60m) for each instance |
| | `POST /object/config` | for the objects the collector says it has no config of |
| each node | `POST /node/stats` | every `--stats-interval` (5m) |
| | `POST /node/system` (asset, then packages), `/node/checks`, `/node/disk` | every `--daily-interval` (24h) |
| | `GET /node/actionq`, `POST /node/actionq/running`, `/done` | when the speaker hears the node has queued actions; nothing is run |

The status of a service changes `--changes-per-hour` times an hour: an
optional resource flapping, a resource failing or coming back, or for a
failover service, a move to another node of its cluster.

## Replayable

Everything is drawn from `--prefix` and `--seed`: the same flags give the same
nodes, ids, services, resources and inventory, and each cluster the same
sequence of events, tick after tick. A larger fleet keeps the objects of a
smaller one. The ramp spreads the start of the clusters over `--ramp`, and the
first resource info and inventory pushes of each node over their interval, as
in a fleet whose agents were not started together.

## Usage

The nodes must be known of the collector to authenticate: `setup` prints the
SQL registering them (`nodes`, `auth_node`, the app), the clusters and services
being made by the workers from the feed. `teardown` removes the fleet and what
the collector made of it (except the stats files of the nodes).

```sh
dbsql() { sudo docker exec -i dev..oc3.container.db sh -c 'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" opensvc'; }

go build -o agent-sim ./tools/agent-sim
./agent-sim plan  --nodes 3000 --nodes-per-cluster 3 --services 15000   # sizes and expected rates
./agent-sim setup --nodes 3000 --nodes-per-cluster 3 --services 15000 | dbsql
./agent-sim run   --nodes 3000 --nodes-per-cluster 3 --services 15000 \
    --feeder http://127.0.0.1:8080 --redis 127.0.0.1:6379 --duration 30m --ramp 5m --name n3000
./agent-sim teardown | dbsql
```

The fleet flags of `setup` and `run` must be the same. The agents date the
lines of the action logs in their local time: run with the TZ of the collector
(`TZ=Europe/Paris`).

On the dev stack, the feeder and redis listen in the network namespace of the
service: run there.

```sh
sudo docker run --rm --network container:dev..oc3.container.0 -e TZ=Europe/Paris \
    -v $PWD/agent-sim:/agent-sim:ro -v $PWD/results:/results -u $(id -u) alpine \
    /agent-sim run --feeder http://127.0.0.1:8080 --redis 127.0.0.1:6379 --results /results ...
```

To exercise every path in a short run, shorten the intervals and raise the
rates: `--info-interval 2m --daily-interval 2m --stats-interval 1m
--changes-per-hour 20 --actions-per-day 300`.

## Results

In `results/<name>/`:

- `run.json`: the flags of the run;
- `samples.csv`: every request, its endpoint, code, duration and size;
- `windows.csv`: by `--report-every` period, the rate, errors and percentiles;
- `queues.csv`: with `--redis`, the length of each worker queue every 5s: a
  queue that grows is a worker not keeping up with the feed;
- `summary.json`: by endpoint, the count, errors, rate and percentiles.

The feeder answers once the data is queued: its latency says little of the
work done. The queues, and the time it takes for a change to reach the
database, say what the workers keep up with.
