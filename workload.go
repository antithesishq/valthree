package main

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/antithesishq/valthree/internal/client"
	"github.com/antithesishq/valthree/internal/proptest"
	"github.com/spf13/cobra"
)

var workloadCmd = &cobra.Command{
	Use:   "workload",
	Short: "Start a continuous testing workload",
	Long:  "Start a continuous testing workload. The workload runs indefinitely, executing a random sequence of commands on a Valthree cluster and periodically verifying that the cluster has maintained strong serializable consistency.",
	Run: func(cmd *cobra.Command, args []string) {
		logger, err := newLogger(cmd.Flags())
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		clusterAddr, err := cmd.Flags().GetString("addr")
		if err != nil {
			logger.Error("cluster addr flag invalid", "err", err)
			os.Exit(1)
		}
		addr, err := net.ResolveTCPAddr("tcp", clusterAddr)
		if err != nil {
			logger.Error("resolve cluster addr failed", "cluster_addr", clusterAddr, "err", err)
			os.Exit(1)
		}
		logger.Info("resolved cluster addr", "cluster_addr", addr)

		// Verify that any flags we'll need later are well-formed.
		checkTimeout, err := cmd.Flags().GetDuration("check-timeout")
		if err != nil {
			logger.Error("check-timeout flag invalid", "err", err)
		}

		pinger := dial(logger, addr) // blocks until cluster is ready
		defer pinger.CloseAndLog(logger)
		logger.Info("setup complete", "cluster_addr", addr)

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

		for {
			select {
			case <-sig:
				os.Exit(0)
			default:
				loadAndVerify(logger, addr, checkTimeout)
			}
		}
	},
}

func loadAndVerify(logger *slog.Logger, addr net.Addr, timeout time.Duration) {
	// Generate a large, randomized workload.
	r := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	workloads := proptest.GenWorkloads(4 /* scale */, r)

	// Run the workload and collect the results.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, workload := range workloads {
		wg.Go(func() {
			client := dial(logger, addr)
			defer client.CloseAndLog(logger)
			<-start
			proptest.RunWorkload(client, workload)
		})
	}
	close(start)
	wg.Wait()

	// Check whether the workload results contain any consistency violations.
	if err := proptest.CheckWorkloads(timeout, workloads); err != nil {
		logger.Error("consistency check failed", "err", err)
		var perr *proptest.Error
		if errors.As(err, &perr) {
			fname := fmt.Sprintf("consistency-failure-%s.html", perr.Key)
			if err := os.WriteFile(fname, perr.Visualization.Bytes(), 0644); err != nil {
				logger.Error("write model visualization failed", "err", err, "key", perr.Key)
			}
		}
	} else {
		logger.Info("consistency check passed")
	}

	// Flush the cluster in preparation for the next workload.
	logger = logger.With("cluster_addr", addr)
	logger.Info("flushing cluster")
	client := dial(logger, addr)
	for {
		if err := client.FlushAll(); err != nil {
			logger.Debug("flush failed", "retry_after", time.Second, "err", err)
			time.Sleep(time.Second)
			continue
		}
		logger.Info("flushed cluster")
		return
	}
}

func dial(logger *slog.Logger, addr net.Addr) *client.Client {
	var usable *client.Client
	for {
		c, err := client.New(addr)
		if err != nil {
			_ = c.Close()
			logger.Debug("dial failed", "retry_after", time.Second, "err", err)
			time.Sleep(time.Second)
			continue
		}
		usable = c
		break
	}
	for {
		err := usable.Ping()
		if err != nil {
			logger.Debug("ping failed", "retry_after", time.Second, "err", err)
			time.Sleep(time.Second)
			continue
		}
		return usable
	}
}

func init() {
	rootCmd.AddCommand(workloadCmd)

	workloadCmd.Flags().String("addr", "valthree:6379", "Valthree cluster address")
	workloadCmd.Flags().Duration("check-timeout", time.Hour, "model checking timeout")
}
