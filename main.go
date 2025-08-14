package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var rootCmd = &cobra.Command{
	Use:   "valthree",
	Short: "A key-value database backed by object storage",
	Long:  "A clustered, Valkey-compatible key-value database backed by object storage.",
}

func init() {
	rootCmd.PersistentFlags().Bool("json", false, "emit logs in JSON")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "emit debug logs")
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func newLogger(flags *pflag.FlagSet) (*slog.Logger, error) {
	level := slog.LevelInfo
	if orFatal(flags.GetBool("verbose")) {
		level = slog.LevelDebug
	}
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
		Level:     level,
	})
	if orFatal(flags.GetBool("json")) {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: false,
			Level:     level,
		})
	}
	return slog.New(handler), nil
}

func orFatal[T any](val T, err error) T {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return val
}
