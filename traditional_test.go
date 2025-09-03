package main_test

import (
	"testing"

	"github.com/antithesishq/valthree/internal/servertest"
	"go.akshayshah.org/attest"
)

func TestTraditional(t *testing.T) {
	// This is a traditional, example-based integration test: it doesn't use
	// property-based testing or Antithesis.
	clients := servertest.NewCluster(t, 1 /* num clients */)
	c := clients[0]

	// SET foo bar == OK
	attest.Ok(t, c.Set("foo", "bar"))

	// GET foo == bar
	val, err := c.Get("foo")
	attest.Ok(t, err)
	attest.Equal(t, val, "bar")

	// DEL foo == OK
	attest.Ok(t, c.Del("foo"))
}
