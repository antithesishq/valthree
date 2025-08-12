package main

import (
	"testing"

	"github.com/antithesishq/valthree/internal/servertest"
	"go.akshayshah.org/attest"
)

func TestSimpleIntegration(t *testing.T) {
	// This is a simple integration test: it doesn't use property-based testing
	// or Antithesis.
	client := servertest.New(t)

	keys, err := client.Keys()
	attest.Ok(t, err)
	attest.Equal(t, keys, []string{})

	attest.Ok(t, client.Set("foo", "bar"))

	val, err := client.Get("foo")
	attest.Ok(t, err)
	attest.Equal(t, val, "bar")

	exists, err := client.Exists("foo")
	attest.Ok(t, err)
	attest.True(t, exists)
}
