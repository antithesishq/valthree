package main_test

import (
	"errors"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/antithesishq/valthree/internal/client"
	"github.com/antithesishq/valthree/internal/proptest"
	"github.com/antithesishq/valthree/internal/servertest"
	"go.akshayshah.org/attest"
)

func TestExample(t *testing.T) {
	// This is a simple integration test: it doesn't use property-based testing
	// or Antithesis.
	clients := servertest.NewCluster(t, 1 /* num clients */)
	c := clients[0]

	// GET foo == ERR
	_, err := c.Get("foo")
	attest.ErrorIs(t, err, client.ErrNotFound)

	// SET foo bar == OK
	attest.Ok(t, c.Set("foo", "bar"))

	// GET foo == bar
	val, err := c.Get("foo")
	attest.Ok(t, err)
	attest.Equal(t, val, "bar")

	// DEL foo == OK
	attest.Ok(t, c.Del("foo"))

	// GET foo == ERR
	_, err = c.Get("foo")
	attest.ErrorIs(t, err, client.ErrNotFound)
}

func TestStrongSerializable(t *testing.T) {
	// This is a property-based test. Rather than testing with hard-coded
	// example inputs, we generate a random workload, execute it, and verify
	// that the results do not violate Valthree's strong serializable
	// consistency guarantee. It's more complex than TestExample, but also far
	// more thorough.
	//
	// This test uses the same proptest package as the Antithesis workload (in
	// workload.go). This is a common pattern: factoring out property-based
	// testing helpers lets developers iterate quickly on their workstations,
	// gaining confidence before kicking off a longer test on the Antithesis
	// platform.
	r := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))

	// First, we generate a random, concurrent workload. The workload is a set of
	// instructions telling each client to perform a series of GET, PUT, and DEL
	// operations on a handful of keys.
	workloads := proptest.GenWorkloads(r)

	// Next, we start a MinIO object storage node, a cluster of Valthree nodes,
	// and a few clients for each node. The servertest package orchestrates this
	// and automatically shuts everything down at the end of the test.
	clients := servertest.NewCluster(t, len(workloads))

	// Then, we run the workload. As the clients execute their assigned
	// operations, they collect timing information and store the result of each
	// command.
	//
	// To increase the chances that multiple clients access the same key at the
	// same time, we block the clients until everyone's ready to start.
	var wg sync.WaitGroup
	start := make(chan struct{})
	logger := servertest.NewLogger(t)
	for i, workload := range workloads {
		wg.Go(func() {
			<-start
			proptest.RunWorkload(logger, clients[i], workload)
		})
	}
	close(start)
	wg.Wait()

	// Using the porcupine linearizability checker, verify that our execution
	// results show that each key is linearizable - so the database as a whole is
	// strong serializable! Etcd, the strong serializable key-value store used by
	// Kubernetes, uses the same linearizability checker.
	timeout := time.Minute
	if deadline, ok := t.Context().Deadline(); ok {
		timeout = time.Until(deadline)
	}
	_, err := proptest.CheckWorkloads(timeout, workloads)
	if attest.Ok(t, err, attest.Sprintf("strong serializability violated")) {
		return
	}
	// Porcupine produces interactive visualizations to help debug any failures.
	if perr := new(proptest.Error); errors.As(err, &perr) {
		const fname = "consistency-failure.html"
		attest.Ok(t, os.WriteFile(fname, perr.Visualization.Bytes(), 0644))
	}
}
