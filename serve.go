package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/antithesishq/valthree/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Valthree server",
	Long:  "Start the Valthree server.",
	Run: func(cmd *cobra.Command, args []string) {
		logger, err := newLogger(cmd.Flags())
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		srv := server.New(server.Config{
			DatabaseName: orFatal(cmd.Flags().GetString("name")),
			MaxItems:     orFatal(cmd.Flags().GetInt("max-keys")),
			S3Endpoint:   orFatal(cmd.Flags().GetString("s3-addr")),
			S3Region:     orFatal(cmd.Flags().GetString("s3-region")),
			S3User:       orFatal(cmd.Flags().GetString("s3-user")),
			S3Password:   orFatal(cmd.Flags().GetString("s3-password")),
			S3Bucket:     orFatal(cmd.Flags().GetString("s3-bucket")),
			S3Timeout:    orFatal(cmd.Flags().GetDuration("s3-timeout")),
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

		addr := orFatal(cmd.Flags().GetString("addr"))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Error("listen failed", "addr", addr, "err", err)
		}

		var wg sync.WaitGroup
		wg.Go(func() {
			logger.Info("starting server", "addr", addr)
			if err := srv.ServeTCP(ln); err != nil {
				logger.Error("serve failed", "err", err)
			}
		})
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
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().String("addr", ":6379", "address to listen on")
	serveCmd.Flags().String("name", "valthree", "database name")
	serveCmd.Flags().Int("max-keys", 16384, "maximum number of stored keys")
	serveCmd.Flags().String("s3-addr", "minio:9000", "object storage address")
	serveCmd.Flags().String("s3-region", "us-east-1", "object storage region")
	serveCmd.Flags().String("s3-bucket", "valthree", "object storage bucket")
	serveCmd.Flags().String("s3-user", "admin", "object storage user")
	serveCmd.Flags().String("s3-pass", "password", "object storage password")
	serveCmd.Flags().Duration("s3-timeout", time.Minute, "object storage timeout")
}
