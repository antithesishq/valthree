package main

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/antithesishq/valthree/internal/server"
)

const defaultHost = "localhost"
const defaultPort = "6379" // Valkey default
const defaultDbName = "valthree"
const defaultMaxItems = 16384 // 2 ** 14
const defaultS3Endpoint = "minio:9000"
const defaultS3Region = "us-east-1"
const defaultS3User = "admin"
const defaultS3Password = "password"
const defaultS3Bucket = "valthree"
const defaultS3Timeout = time.Minute

// FIXME: use pflag instead
var (
	host       = defaultHost
	port       = defaultPort
	dbName     = defaultDbName
	maxItems   = defaultMaxItems
	s3Endpoint = defaultS3Endpoint
	s3Region   = defaultS3Region
	s3User     = defaultS3User
	s3Password = defaultS3Password
	s3Bucket   = defaultS3Bucket
	s3Timeout  = defaultS3Timeout
)

func main() {
	logger := slog.Default()
	srv := server.New(server.Config{
		DatabaseName: dbName,
		MaxItems:     maxItems,
		S3Endpoint:   s3Endpoint,
		S3Region:     s3Region,
		S3User:       s3User,
		S3Password:   s3Password,
		S3Bucket:     s3Bucket,
		S3Timeout:    s3Timeout,
	})

	for {
		if err := srv.EnsureBucketExists(); err != nil {
			backoff := time.Second
			logger.Info("bucket not ready", "err", err, "retry_after", backoff)
			time.Sleep(backoff)
			continue
		}
		break
	}

	addr := net.JoinHostPort(host, port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen failed", "addr", addr, "err", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("starting server", "addr", addr)
		if err := srv.ServeTCP(ln); err != nil {
			logger.Error("serve failed", "err", err)
		}
	}()
	defer func() {
		if err := srv.Close(); err != nil {
			logger.Error("close failed", "err", err)
			os.Exit(1)
		}
		wg.Wait()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
