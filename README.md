# valthree = valkey + S3

**Valthree is a Valkey- and Redis-compatible database backed by S3.**
Unlike traditional implementations, Valthree servers are stateless and designed to run behind a load balancer.
Clusters offer [strong serializable consistency][strong-serializable] and the same 99.999999999% durability as S3.

Applications connect to a Valthree cluster using any Valkey or Redis client library.
Clusters support the `GET`, `SET`, `DEL`, `FLUSHALL`, `PING`, and `QUIT` commands.

> [!IMPORTANT]
> **We built Valthree to show off [Antithesis][antithesis], our platform for testing distributed systems.**
> Rather than prioritizing performance or feature parity with Valkey, we've kept this project simple: it's real enough to have bugs, but small enough to understand quickly.
> Please don't rely on Valthree in production!
>
> For more on Valthree's design, testing strategy, and Antithesis integration, read on.
> If you'd rather see a mission-critical distributed database tested with Antithesis, head over to [etcd][etcd-antithesis].

<!-- TODO: embed a video covering all of this -->

## :triangular_ruler: Design

Valthree clusters persist the whole key-value database as a *single* JSON file in object storage.
To preserve consistency, clusters use optimistic concurrency control:

- To handle `SET` and `DEL` commands, servers read the database file from object storage, modify it, and write it back with the `If-Match` header.
  If another process has modified the database during the read-modify-write cycle, the database's ETag changes, the write fails, and the client receives an error.
- To handle `GET` commands, servers read the database file from object storage (without any caching).
- To handle `FLUSHALL` commands, servers delete the database file.

This is terrible for performance &mdash; all writes conflict with each other! &mdash; but it's simple enough to implement in [just two files](./internal/server).
Keeping the implementation small lets us focus on testing.

```

    ┌──────────┐                                  ┌─────────┐
  ┌──────────┐ │              reads               │         │
┌──────────┐ │ │  ◄─────────────────────────────  │   S3    │
│ valthree │ │ │                                  │ db.json │
│          │ │─┘  ─────────────────────────────►  │         │
│          │─┘         conditional writes         │         │
└──────────┘                                      └─────────┘

```

## :white_check_mark: Testing

### Example-based testing

Valthree has only one traditional, example-based unit test: `TestExample` in [`main_test.go`](./main_test.go).
It's straightforward but limited.
There's just one server and one client, so it doesn't test our optimistic concurrency control.
It *certainly* doesn't test whether Valthree delivers on its consistency and durability claims in fault-prone production environments.
Even without Antithesis, we can do much better.

### Property-based testing

`TestStrongSerializable`, also in [`main_test.go`](./main_test.go), is much more effective.
It uses a cluster of Valthree servers and multiple clients per server.
And rather than executing a fixed sequence of operations, each client runs a randomly-generated workload and records the results.
Compared to our example-based test, property-based testing exercises Valthree much more thoroughly.

But with a randomly-generated, concurrent workload, it's harder to verify Valthree's correctness.
Consider two clients, each issuing a command at the same time: one sends `SET foo bar`, and the other sends `SET foo quux`.
Both `SET`s succeed.
What should subsequent `GET foo` calls return?
It's hard to say &mdash; it depends on the vagaries of thread scheduling, network timing, and many other factors.
From our test's perspective, either `bar` or `quux` is correct.
To handle this uncertainty, Valthree's tests use [porcupine][], an open source linearizability checker.
We model each key as a set of possible values, updating the possibilities with each operation.
Porcupine checks the clients' observations against this model.
This is computationally difficult (it's NP-hard!), but porcupine uses some [fancy tricks][p-compositionality] to make checking as fast as possible.

The machinery for this style of testing is in [`internal/proptest`](./internal/proptest/).
The three key pieces &mdash; workload generation, workload execution, and model checking &mdash are implemented as `GenWorkloads`, `RunWorkload`, and `CheckWorkloads`.
No matter what sort of distributed system you're testing, this generate-execute-check pattern is applicable.
Systems with strong invariants lend themselves to simpler checkers.
For example, in any double-entry ledger, credits and debits must always be balanced.
Systems without strong invariants, like key-value stores, need more complex checkers.
Luckily, there are many good open source checkers available!

So far, none of Valthree's testing depends on Antithesis.
All the tests run locally, so we can iterate quickly and detect obvious bugs.
But we still haven't tested that Valthree maintains its guarantees when the environment isn't quite so gentle.

### Antithesis

<!-- TODO -->


## :hearts: Legal

Offered under the [MIT License](LICENSE.md).

[antithesis]: https://antithesis.com
[etcd-antithesis]: https://github.com/etcd-io/etcd/tree/main/tests/antithesis
[strong-serializable]: https://antithesis.com/resources/reliability_glossary/#strong-serializable
[stable-go]: https://golang.org/doc/devel/release#policy
[porcupine]: https://github.com/anishathalye/porcupine
[p-compositionality]: https://arxiv.org/pdf/1504.00204
