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
	dial := servertest.New(t)
	c := dial()

	_, err := c.Get("foo")
	attest.ErrorIs(t, err, client.ErrNotFound)

	attest.Ok(t, c.Set("foo", "bar"))

	val, err := c.Get("foo")
	attest.Ok(t, err)
	attest.Equal(t, val, "bar")

	attest.Ok(t, c.Del("foo"))

	_, err = c.Get("foo")
	attest.ErrorIs(t, err, client.ErrNotFound)
}

func TestStrongSerializable(t *testing.T) {
	// This is a property-based test. Rather than testing with hard-coded
	// example inputs, we generate a random workload, execute it, and verify
	// that the results do not violate Valthree's strong serializable
	// consistency guarantee. It's more complex than TestExample, but also far
	// more thorough.
	r := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	workloads := proptest.GenWorkloads(1 /* scale */, r)
	dial := servertest.New(t)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, workload := range workloads {
		wg.Go(func() {
			client := dial()
			<-start
			proptest.RunWorkload(client, workload)
		})
	}
	close(start)
	wg.Wait()
	timeout := time.Minute
	if deadline, ok := t.Context().Deadline(); ok {
		timeout = time.Until(deadline)
	}
	err := proptest.CheckWorkloads(timeout, workloads)
	if err != nil {
		t.Log(err)
		t.Fail()
		var perr *proptest.Error
		if errors.As(err, &perr) {
			if err := os.WriteFile("consistency-failure.html", perr.Visualization.Bytes(), 0644); err != nil {
				t.Logf("write porcupine visualization: %v", err)
			}
		}
	}
}
