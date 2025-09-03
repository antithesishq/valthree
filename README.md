# valthree = valkey + S3

**Valthree is a clustered, Valkey- and Redis-compatible database backed by S3.**
Clusters offer [strong serializable consistency][strong-serializable] and have the same 99.999999999% durability as S3.

Applications connect to a Valthree cluster using any Valkey or Redis client library.
Clusters support the `GET`, `SET`, `DEL`, `FLUSHALL`, `PING`, and `QUIT` commands.

> [!IMPORTANT]
> **We built Valthree to show off [Antithesis][antithesis], our platform for testing distributed systems.**
> Rather than prioritizing performance or feature parity with Valkey, we've kept this project simple: it's real enough to have bugs, but small enough to understand quickly.
> Please don't rely on Valthree in production!
>
> For more on Valthree's design, testing strategy, and Antithesis integration, read on.
> If you'd rather see a mission-critical distributed database tested with Antithesis, head over to [etcd][etcd-antithesis].

<!-- TODO: embed a video covering this and showing reports -->

## :triangular_ruler: Design

Valthree clusters persist the whole key-value database as a *single* JSON file in object storage.
Clusters preserve consistency with optimistic concurrency control:

- To handle `SET` and `DEL` commands, servers read the database file from object storage, modify it, and write it back with the `If-Match` header.
  If another process has modified the file during the read-modify-write cycle, the file's ETag changes, the write fails, and the client receives an error.
- To handle `GET` commands, servers read the file from object storage (without any caching).
- To handle `FLUSHALL` commands, servers delete the database file.

```

                       ┌──────────┐                                    ┌─────────┐
                     ┌──────────┐ │                                    │         │
─── 1) SET a b ──┐ ┌──────────┐ │ │ ┌──── 2) ETag:123 {"a": "x"} ────  │   S3    │
                 │ │          │ │ │ │                                  │ db.json │
◄──── 4) OK ─────┘ │ valthree │ │─┘ └─ 3) If-Match:123 {"a": "b"} ──►  │         │
                   │          │─┘                                      │         │
                   └──────────┘                                        └─────────┘

```

This design is terrible for performance &mdash; all writes conflict with each other! &mdash; but it's simple enough to implement in [just two files](./internal/server).
Keeping the implementation small lets us focus on testing.

## :bug: Testing with Antithesis

Valthree uses [Antithesis][antithesis] to make sure that clusters remain consistent &mdash;
even in the face of faulty networks, unreliable disks, unsynchronized clocks, and all the other indignities of production environments.
Rather than maintaining a tightly-coupled, ever-growing pile of traditional tests, Antithesis lets us thoroughly test Valthree with just one black-box test.

Valthree's tests rely on [_property-based testing_][pbt].
Rather than running a hard-coded series of commands and then asserting the exact state of the database, Valthree's tests spin up multiple clients, each of which executes a randomly-generated workload.
Periodically, the tests verify that the clients haven't observed any inconsistencies.
When run in Antithesis's deterministic environment and driven by our autonomous exploration engine, this one test finds Valthree's deepest bugs, makes them perfectly reproducible, and even lets us interactively debug failures.

The best places to start browsing Valthree's code are the entrypoints for [the server](./server.go) and [the Antithesis tests](./workload.go).
On each commit, [a Github Action](./.github/workflows/ci.yaml) builds them into a container (defined in [Dockerfile.valthree](./Dockerfile.valthree)) and pushes them to Antithesis's artifact registry.
The same Github Action also pushes a [Docker Compose file](./docker-compose.yaml) that stiches together MinIO, a three-node Valthree cluster, and the test workload.
Antithesis spins up the whole system, thoroughly explores its behavior, and produces a report of any failures.

Valthree's code includes examples of:

- [Using the Antithesis Github Action](./.github/workflows/ci.yaml) ([docs][action-docs])
- [Defining custom properties][assertions] with the Antithesis SDK ([docs][properties-docs])
- [Instrumenting][instrumentation] a Go binary for coverage-assisted exploration ([docs][instrumentation-docs])
- [Emulating an AWS service][minio] in Antithesis ([docs][3p-docs])
- Maintaining a [local test workload](./main_test.go) for quick iteration (though it doesn't have fault injection or intelligent exploration)

See the [full Antithesis documentation][docs] for more information.
If you'd prefer a live demo, [let us know][book-demo]!

## :hearts: Legal

Offered under the [MIT License](LICENSE.md).

[3p-docs]: https://antithesis.com/docs/reference/dependencies/
[action-docs]: https://antithesis.com/docs/using_antithesis/ci/#github-actions
[antithesis]: https://antithesis.com
[assertions]: https://github.com/search?q=repo%3Aantithesishq%2Fvalthree+%22assert.%22&type=code
[book-demo]: https://antithesis.com/book-a-demo/
[docs]: https://antithesis.com/docs/
[etcd-antithesis]: https://github.com/etcd-io/etcd/tree/main/tests/antithesis
[instrumentation-docs]: https://antithesis.com/docs/instrumentation/
[instrumentation]: https://github.com/search?q=repo%3Aantithesishq%2Fvalthree%20antithesis-go-instrumentor&type=code
[minio]: https://github.com/search?q=repo%3Aantithesishq%2Fvalthree+%22container_name%3A+minio%22&type=code
[pbt]: https://antithesis.com/resources/property_based_testing/
[properties-docs]: https://antithesis.com/docs/instrumentation/
[strong-serializable]: https://antithesis.com/resources/reliability_glossary/#strong-serializable
