// Package servertest provides utilities for testing the Valthree servers.
package servertest

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/antithesishq/valthree/internal/client"
	"github.com/antithesishq/valthree/internal/server"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"go.akshayshah.org/attest"
)

// New starts a new Valthree server on an ephemeral port and returns a
// ready-to-use client. The server and client are automatically cleaned up when
// the test completes.
//
// Under the hood, New uses testcontainers to start a MinIO container for the
// Valthree server's storage. The MinIO container is also cleaned up when the test
// completes.
func New(tb testing.TB) *client.Client {
	tb.Helper()

	const user, password = "admin", "password"
	tb.Logf("starting MinIO  container")
	mc, err := minio.Run(
		tb.Context(),
		"minio/minio:RELEASE.2025-07-23T15-54-02Z",
		minio.WithUsername(user),
		minio.WithPassword(password),
	)
	attest.Ok(tb, err, attest.Sprint("start MinIO container"))
	tb.Logf("MinIO  container started")
	addr, err := mc.ConnectionString(tb.Context())
	attest.Ok(tb, err, attest.Sprint("get MinIO conn str"))

	srv := server.New(server.Config{
		DatabaseName: "test",
		MaxItems:     1024,
		S3Endpoint:   fmt.Sprintf("http://%s", addr),
		S3Region:     "us-east-1",
		S3User:       user,
		S3Password:   password,
		S3Bucket:     "valthree",
		S3Timeout:    time.Second,
	})
	attest.Ok(tb, srv.EnsureBucketExists(), attest.Sprint("create bucket"))

	ln, err := net.Listen("tcp", "localhost:0") // closed by redcon server
	attest.Ok(tb, err, attest.Sprint("listen on ephemeral port"))

	var wg sync.WaitGroup
	wg.Add(1)
	tb.Logf("starting redcon server")
	go func() {
		defer wg.Done()
		attest.Ok(tb, srv.ServeTCP(ln), attest.Sprint("redcon serve"))
	}()
	tb.Cleanup(func() {
		attest.Ok(tb, srv.Close(), attest.Sprint("redcon close"))
		wg.Wait()
	})

	client, err := client.New(ln.Addr())
	attest.Ok(tb, err, attest.Sprint("client dial"))
	tb.Cleanup(func() {
		attest.Ok(tb, client.Close(), attest.Sprint("client close"))
	})
	tb.Logf("attempting to ping redcon server")
	for {
		if err := client.Ping(); err == nil {
			tb.Logf("redcon server ping successful")
			return client
		}
		backoff := 100 * time.Millisecond
		tb.Logf("redcon server not ready, retrying after %v", backoff)
		time.Sleep(backoff)
	}
}
