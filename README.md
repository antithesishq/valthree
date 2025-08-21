# valthree = valkey + S3

**Valthree is a Valkey- and Redis-compatible database backed by S3.**
Unlike traditional implementations, Valthree servers are stateless and designed to run behind a load balancer.
Clusters offer [strong serializable consistency][strong-serializable] and only acknowledge writes after they've been persisted to object storage, so Valthree is usable as both a persistent cache and a primary database.

Applications connect to a Valthree cluster using any Valkey or Redis client library.
Clusters support the `GET`, `SET`, `DEL`, `FLUSHALL`, `PING`, and `QUIT` commands.

## :bulb: Motivation

**We built Valthree to show off [Antithesis][antithesis], our platform for testing distributed systems.**
So rather than maximizing performance, minimizing operating costs, or implementing a lot of features, we've intentionally kept this project simple: it's real enough to have bugs, but small enough to understand quickly.
Please don't rely on Valthree in production!

For more on Valthree's design, testing strategy, and Antithesis integration, read on.
If you'd rather see a mission-critical distributed database tested with Antithesis, head over to [etcd][etcd-antithesis].

## :pencil: Design

Valthree clusters persist the whole key-value database as a single JSON file in object storage.
To preserve consistency, clusters use conditional writes and optimistic concurrency control:

- Servers handle `SET` and `DEL` by reading the database file from object storage, modifying it, and then writing it back with the `If-Match` header.
  If the database was modified after the initial read, the write fails and the client receives an error.
- `GET`s are served directly from object storage, without any caching.
- `FLUSHALL` deletes the database file.

This is terrible for performance &mdash; writes to the whole database are serialized! &mdash; but it's simple.
The whole implementation is in [`internal/server`](./internal/server).
Keeping the implementation simple lets us focus on testing.

```

    ┌──────────┐                                  ┌─────────┐
  ┌──────────┐ │                                  │         │
┌──────────┐ │ │  ─────────────────────────────►  │   S3    │
│ valthree │ │ │   reads & conditional writes     │ db.json │
│          │ │─┘  ◄─────────────────────────────  │         │
│          │─┘                                    │         │
└──────────┘                                      └─────────┘

```

## :white_check_mark: Testing

### Traditional testing

Valthree has only one traditional, example-based unit test: `TestExample` in [`main_test.go`](./main_test.go).
It's straightforward, but limited.
There's just one client, so it doesn't test our optimistic concurrency control.
It certainly doesn't test how Valthree behaves in fault-prone production environments.

<!-- TODO: continue, working up to property-based testing and Antithesis -->


## :hearts: Legal

Offered under the [MIT License](LICENSE.md).

[antithesis]: https://antithesis.com
[etcd-antithesis]: https://github.com/etcd-io/etcd/tree/main/tests/antithesis
[strong-serializable]: https://antithesis.com/resources/reliability_glossary/#strong-serializable
[stable-go]: https://golang.org/doc/devel/release#policy
