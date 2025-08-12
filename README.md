# valthree

Valthree is a highly-available Valkey- and Redis-compatible database backed by S3.
Clusters are strongly consistent and only acknowledge writes after they've been persisted to object storage.
Applications may connect to a Valthree cluster using any Valkey or Redis client library.

We built Valthree to show off [Antithesis][antithesis], our platform for testing distributed systems.
So while Valthree is rigorously tested for correctness, it doesn't support the full Valkey API or optimize for throughput, latency, or cost.
Instead, we've intentionally kept this project small and simple.

To see a mission-critical distributed database tested with Antithesis, head over to [etcd][etcd-antithesis].

## Design

<!-- TODO: explain basic read/write flow -->

```

    ┌──────────┐                                  ┌──────────┐
  ┌──────────┐ │                                  │          │
┌──────────┐ │ │  ─────────────────────────────►  │          │
│ valthree │ │ │   reads & conditional writes     │    S3    │
│          │ │─┘  ◄─────────────────────────────  │          │
│          │─┘                                    │          │
└──────────┘                                      └──────────┘

```

## Testing

<!-- TODO: document testing approach -->

## Run Valthree locally

<!-- TODO: Add usage instructions -->

```
docker compose up
```

## Status: unstable

Valthree is a demonstration project.
We don't guarantee backward compatibility, don't tag releases, don't prioritize observability, and don't offer any commercial support.
Unless your needs are very unusual, you shouldn't use Valthree in production.

## Legal

Offered under the [MIT License](LICENSE.md).


[antithesis]: https://antithesis.com
[etcd-antithesis]: https://github.com/etcd-io/etcd/tree/main/tests/antithesis
