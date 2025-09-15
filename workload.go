package main

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/antithesishq/antithesis-sdk-go/lifecycle"
	"github.com/antithesishq/valthree/internal/client"
	"github.com/antithesishq/valthree/internal/proptest"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(workloadCmd)

	workloadCmd.Flags().StringSlice("addrs", []string{":6379"}, "Valthree cluster address(es)")
	workloadCmd.Flags().Duration("check-timeout", time.Hour, "model checking timeout")
	workloadCmd.Flags().String("artifacts", ".", "directory for storing debugging artifacts")
}

var workloadCmd = &cobra.Command{
	Use:   "workload",
	Short: "Start a continuous workload exercising a Valthree cluster",
	Run: func(cmd *cobra.Command, args []string) {
		// The entry point for the Antithesis workload, which runs indefinitely.
		// First, validate the user-supplied flags. Because we're optimizing for
		// brevity, we simply crash when flags are invalid.
		logger := orFatal(newLogger(cmd.Flags()))
		clusterAddrs := orFatal(cmd.Flags().GetStringSlice("addrs"))
		checkTimeout := orFatal(cmd.Flags().GetDuration("check-timeout"))
		artifactDir := orFatal(cmd.Flags().GetString("artifacts"))

		// Before injecting faults, the Antithesis platform lets us verify that our
		// system is up and running. We'll check the cluster by waiting for each
		// server to respond to a PING.
		addrs := make([]net.Addr, len(clusterAddrs))
		for i, serverAddr := range clusterAddrs {
			logger := logger.With("server_addr", serverAddr)
			addr, err := net.ResolveTCPAddr("tcp", serverAddr)
			if err != nil {
				logger.Error("server addr misconfigured", "err", err)
				os.Exit(1)
			}
			logger.Debug("resolved server addr")

			pinger := dial(logger, addr) // blocks until cluster is ready
			logger.Debug("pinged server")
			pinger.CloseAndLog(logger)
			addrs[i] = addr
		}
		// The cluster is up! Using the Antithesis SDK, we tell the platform that
		// we're ready for fault injection.
		logger.Info("setup complete", "cluster_addrs", addrs)
		lifecycle.SetupComplete(map[string]any{"cluster_addrs": addrs})

		// Until the workload gets a signal to stop, exercise the cluster. Each
		// iteration generates a random, concurrent workload, records the results,
		// and verifies that the cluster is strict serializable.
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		var iterations int
		for {
			select {
			case <-sig:
				os.Exit(0)
			default:
				exerciseAndVerify(iterations, logger, addrs, checkTimeout, artifactDir)
				iterations++
			}
		}
	},
}

func exerciseAndVerify(
	iteration int,
	logger *slog.Logger,
	addrs []net.Addr,
	timeout time.Duration,
	artifactDir string,
) {
	seeds := []uint64{rand.Uint64(), rand.Uint64()}
	logger = logger.With("pcg_seeds", seeds, "cluster_addrs", addrs)

	// Before running this workload, return the cluster to a known state by
	// flushing it. This prevents an unclean shutdown from poisoning subsequent
	// runs.
	logger.Debug("flushing cluster")
	client := dial(logger, addrs[0])
	for {
		if err := client.FlushAll(); err != nil {
			logger.Debug("flush failed", "retry_after", time.Second, "err", err)
			time.Sleep(time.Second)
			continue
		}
		logger.Debug("flushed cluster")
		break
	}

	// Next, generate a concurrent, randomized workload. The workload is a set of
	// instructions, telling each client to execute a series of GET, PUT, and DEL
	// commands on a small set of keys.
	logger.Debug("generating new workload")
	r := rand.New(rand.NewPCG(seeds[0], seeds[1]))
	workloads := proptest.GenWorkloads(r)
	// In each test run, start without concurrency. This is purely for
	// demonstration purposes - real workloads don't need this!
	const serialIterations = 16
	if iteration < serialIterations && len(workloads) > 1 {
		workloads = workloads[:1]
	}
	if iteration == serialIterations {
		logger.Info("allowing concurrent workloads")
	}

	// Run the workload, recording the timing and result of each operation. To
	// maximize concurrent work, we block each client until all the clients are
	// ready to begin.
	logger.Debug("running workload")
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, workload := range workloads {
		wg.Go(func() {
			addr := addrs[i%len(addrs)]
			logger := logger.With("client_id", i, "addr", addr)
			client := dial(logger, addr)
			defer client.CloseAndLog(logger)
			<-start
			proptest.RunWorkload(logger, client, workload)
		})
	}
	close(start)
	wg.Wait()
	logger.Debug("workload complete")

	// We've run the workload and collected the results. Using the porcupine
	// linearizability checker, verify that the operations on each key are
	// linearizable - and therefore, that the Valthree key-value store is strong
	// serializable. (Etcd, the strong serializable key-value store at the heart
	// of Kubernetes, also uses porcupine to check linearizability!)
	progress, err := proptest.CheckWorkloads(timeout, workloads)
	if err != nil {
		// Antithesis reports may include debugging artifacts. In this case,
		// porcupine produces an interactive visualization of the consistency bug
		// which we'd like to surface.
		var perr *proptest.Error
		if errors.As(err, &perr) {
			fname := fmt.Sprintf("consistency-failure-%s.html", perr.Key)
			fpath := filepath.Join(artifactDir, fname)
			if err := os.WriteFile(fpath, perr.Visualization.Bytes(), 0644); err != nil {
				logger.Error("write model visualization failed", "err", err, "key", perr.Key)
			}
		}
		// Using the Antithesis SDK, tell the platform that we've violated a
		// critical system property. Unreachable is the simplest assertion, so it
		// just takes a message and loosely-typed details.
		//
		// If integrating the SDK is difficult, Antithesis can also look for the
		// presence or absence of particular log lines.
		assert.Unreachable(
			"Database is strong serializable", // appears as-is in Antithesis reports
			map[string]any{"error": err.Error()},
		)
		logger.Error("strong serializability violated", "err", err)
	} else {
		percent := strconv.FormatFloat(100*progress, 'f', 1 /* precision */, 64 /* bitsize */)
		logger.Info("strong serializability verified", "percent_success", percent)
	}

}

func dial(logger *slog.Logger, addr net.Addr) *client.Client {
	var usable *client.Client
	for {
		c, err := client.New(addr)
		if err != nil {
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
