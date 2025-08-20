// Package servertest provides utilities for testing the Valthree servers.
package servertest

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/antithesishq/valthree/internal/client"
	"github.com/antithesishq/valthree/internal/server"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"go.akshayshah.org/attest"
)

// NewCluster creates a Valthree cluster and returns ready-to-use clients. The
// clients, Valthree servers, and backing MinIO storage are automatically
// cleaned up when the test completes. As long as numClients is greater than
// one, the Valthree cluster has multiple nodes.
func NewCluster(tb testing.TB, numClients int) []*client.Client {
	tb.Helper()
	attest.True(tb, numClients > 0, attest.Sprintf("num clients must be positive"))

	const user, password = "admin", "password"
	// The MinIO testcontainers module includes verbose test logs by default.
	mc, err := minio.Run(
		tb.Context(),
		"minio/minio:RELEASE.2025-07-23T15-54-02Z",
		minio.WithUsername(user),
		minio.WithPassword(password),
	)
	attest.Ok(tb, err, attest.Sprint("start MinIO container"))
	addr, err := mc.ConnectionString(tb.Context())
	attest.Ok(tb, err, attest.Sprint("get MinIO conn str"))

	numServers := 1
	if numClients > 1 {
		numServers = numClients / 2
	}

	logger := NewLogger(tb)
	serverAddrs := make([]net.Addr, numServers)
	for i := range serverAddrs {
		srv := server.New(server.Config{
			DatabaseName: "test",
			MaxItems:     1024,
			S3Endpoint:   fmt.Sprintf("http://%s", addr),
			S3Region:     "us-east-1",
			S3User:       user,
			S3Password:   password,
			S3Bucket:     "valthree",
			S3Timeout:    time.Second,
		}, NewLogger(tb))

		ln, err := net.Listen("tcp", "localhost:0") // closed by redcon server
		attest.Ok(tb, err, attest.Sprint("listen on ephemeral port"))

		var wg sync.WaitGroup
		logger.Debug("starting redcon server", "server_id", i, "addr", ln.Addr())
		wg.Go(func() {
			attest.Ok(tb, srv.ServeTCP(ln), attest.Sprint("redcon serve"))
		})
		tb.Cleanup(func() {
			attest.Ok(tb, srv.Close(), attest.Sprint("redcon close"))
			wg.Wait()
		})
		serverAddrs[i] = ln.Addr()
	}

	clients := make([]*client.Client, numClients)
	for i := range clients {
		addr := serverAddrs[i%len(serverAddrs)]
		client, err := client.New(addr)
		attest.Ok(tb, err, attest.Sprint("client dial"))
		tb.Cleanup(func() {
			attest.Ok(tb, client.Close(), attest.Sprint("client close"))
		})
		for {
			if err := client.Ping(); err == nil {
				break
			}
			backoff := 100 * time.Millisecond
			logger.Debug("redcon server not ready", "addr", addr, "retry_after", backoff)
			time.Sleep(backoff)
		}
		clients[i] = client
	}
	return clients
}

// NewLogger creates a structured logger that writes to the supplied
// testing.TB.
func NewLogger(tb testing.TB) *slog.Logger {
	handler := slog.NewTextHandler(tb.Output(), &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
	})
	return slog.New(handler)
}
