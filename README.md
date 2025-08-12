# valkey + S3 = valthree

Valthree is a Valkey- and Redis-compatible database backed by S3.
Unlike traditional implementations, Valthree servers are stateless and designed to run behind a load balancer.
Clusters are strongly consistent and only acknowledge writes after they've been persisted to object storage.

Applications connect to a Valthree cluster using any Valkey or Redis client library.
Valthree supports the `GET`, `SET`, `DEL`, `EXISTS`, `KEYS *`, `FLUSHALL`, `PING`, and `QUIT` commands.

## Motivation

We built Valthree to show off [Antithesis][antithesis], our platform for testing distributed systems.
So rather than maximizing performance, minimizing operating costs, or implementing the full Valkey API, we've intentionally kept this project simple:
it's real enough to have bugs, but small enough to understand quickly.

For more on Valthree's design, testing strategy, and Antithesis integration, read on.
If you'd rather see a mission-critical distributed database tested with Antithesis, head over to [etcd][etcd-antithesis].

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
