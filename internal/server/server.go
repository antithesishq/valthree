// Package server provides the Valthree server.
package server

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/antithesishq/valthree/internal/op"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/tidwall/redcon"
)

// Config bundles the primitive values that configure a Valthree server.
type Config struct {
	DatabaseName string
	MaxItems     int

	S3Endpoint string
	S3Region   string
	S3Bucket   string
	S3User     string
	S3Password string
	S3Timeout  time.Duration
}

// Server is the Valthree server: a clustered, Valkey-compatible key-value
// store backed by object storage.
type Server struct {
	maxItems int
	store    storage

	mu    sync.Mutex
	close func() error
}

// New constructs a Server.
//
// Before returning, it ensures that the object storage bucket is created and
// ready to use; under adversarial conditions, it will retry bucket creation
// indefinitely.
func New(cfg Config, logger *slog.Logger) *Server {
	s3client := s3.New(s3.Options{
		Region:                     cfg.S3Region,
		BaseEndpoint:               aws.String(cfg.S3Endpoint),
		DefaultsMode:               aws.DefaultsModeStandard,
		Credentials:                credentials.NewStaticCredentialsProvider(cfg.S3User, cfg.S3Password, "" /* session */),
		UsePathStyle:               true,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenSupported,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenSupported,
		HTTPClient: &http.Client{
			Transport: &http.Transport{},
		},
	})
	store := storage{
		timeout: cfg.S3Timeout,
		bucket:  cfg.S3Bucket,
		name:    cfg.DatabaseName,
		client:  s3client,
	}
	for {
		logger := logger.With("bucket", cfg.S3Bucket)
		if err := store.EnsureBucketExists(); err != nil {
			backoff := time.Second
			logger.Error("bucket not ready", "err", err, "retry_after", backoff)
			time.Sleep(backoff)
			continue
		}
		logger.Info("bucket ready")
		break
	}

	return &Server{
		maxItems: cfg.MaxItems,
		store: storage{
			timeout: cfg.S3Timeout,
			bucket:  cfg.S3Bucket,
			name:    cfg.DatabaseName,
			client:  s3client,
		},
	}
}

// ServeTCP accepts connections and serves Valkey requests.
func (s *Server) ServeTCP(ln net.Listener) error {
	rs := redcon.NewServerNetwork("tcp", ln.Addr().String(), s.handle, s.accept, s.onClosed)
	s.mu.Lock()
	s.close = rs.Close
	s.mu.Unlock()
	return rs.Serve(ln)
}

// Close shuts the server down.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.close == nil {
		return nil
	}
	return s.close()
}

func (s *Server) handle(conn redcon.Conn, cmd redcon.Command) {
	name := op.New(cmd.Args[0])
	var args []string
	if len(cmd.Args) > 1 {
		args = make([]string, 0, len(cmd.Args))
		for _, arg := range cmd.Args[1:] {
			args = append(args, string(arg))
		}
	}
	switch name {
	case op.Get:
		s.get(conn, args)
	case op.Set:
		s.set(conn, args)
	case op.Del:
		s.del(conn, args)
	case op.FlushAll:
		s.flushAll(conn, args)
	case op.Ping:
		s.ping(conn, args)
	case op.Quit:
		s.quit(conn, args)
	default:
		conn.WriteError(fmt.Sprintf("ERR unknown command '%s'", name))
	}
}

func (s *Server) accept(conn redcon.Conn) bool {
	return true
}

func (s *Server) onClosed(conn redcon.Conn, err error) {
}

func (s *Server) get(conn redcon.Conn, args []string) {
	if len(args) != 1 {
		writeErrArity(conn, op.Get)
		return
	}

	items, err := s.store.GetDB()
	if err != nil {
		writeErr(conn, err)
		return
	}
	val, ok := items[args[0]]
	if !ok {
		conn.WriteNull()
		return
	}
	if val == "" {
		// See set: explicitly storing empty values is forbidden.
		writeErr(conn, fmt.Errorf("database contains empty value for string %s", args[0])) // unreachable
		return
	}
	conn.WriteBulkString(val)
}

func (s *Server) set(conn redcon.Conn, args []string) {
	if len(args) != 2 {
		writeErrArity(conn, op.Set)
		return
	}
	// Valkey allows SET'ing values to the empty string, but this makes our test
	// model more complex - we can't model the allowable values for a key as a
	// set of strings, because we don't have a value to represent the key being
	// absent. This is a demo project, so we'll disallow empty values to keep
	// the model simple.
	if args[1] == "" {
		writeErr(conn, fmt.Errorf("empty value"))
		return
	}

	_, err := s.store.MutateDB(func(items map[string]string) (int, error) {
		if len(items) >= s.maxItems {
			return 0, fmt.Errorf("at max capacity of %d keys", s.maxItems)
		}
		items[args[0]] = args[1]
		return 0, nil // int doesn't matter
	})
	if err != nil {
		writeErr(conn, err)
		return
	}
	conn.WriteString("OK")

}

func (s *Server) del(conn redcon.Conn, args []string) {
	// Valkey allows DEL'ing multiple keys in one call, but that makes it harder
	// to model the DB as a collection of independent registers. To keep this
	// project simple, restrict DEL to one key at a time.
	if len(args) != 1 {
		writeErrArity(conn, op.Del)
		return
	}

	n, err := s.store.MutateDB(func(items map[string]string) (int, error) {
		_, ok := items[args[0]]
		delete(items, args[0])
		if ok {
			return 1, nil
		}
		return 0, nil
	})

	if err != nil {
		writeErr(conn, err)
		return
	}
	conn.WriteInt(n)
}

func (s *Server) flushAll(conn redcon.Conn, args []string) {
	if len(args) > 0 {
		writeErrArity(conn, op.FlushAll)
		return
	}

	_, err := s.store.MutateDB(func(items map[string]string) (int, error) {
		clear(items)
		return 0, nil
	})
	if err != nil {
		writeErr(conn, err)
		return
	}
	conn.WriteString("OK")
}

func (s *Server) ping(conn redcon.Conn, args []string) {
	conn.WriteString("PONG")
}

func (s *Server) quit(conn redcon.Conn, args []string) {
	conn.WriteString("OK")
	conn.Close()
}

func writeErrArity(conn redcon.Conn, op op.Op) {
	conn.WriteError(fmt.Sprintf("ERR wrong number of arguments for '%s' command", op))
}

func writeErr(conn redcon.Conn, err error) {
	conn.WriteError(fmt.Sprintf("ERR %v", err))
}
