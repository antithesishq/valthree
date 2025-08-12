package server

import (
	"fmt"
	"maps"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/antithesishq/valthree/internal/op"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/tidwall/redcon"
)

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

type Server struct {
	maxItems int
	storage  storage
	close    func() error // set in ServeTCP
}

func New(cfg Config) *Server {
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
	return &Server{
		maxItems: cfg.MaxItems,
		storage: storage{
			timeout: cfg.S3Timeout,
			bucket:  cfg.S3Bucket,
			name:    cfg.DatabaseName,
			client:  s3client,
		},
	}
}

func (s *Server) EnsureBucketExists() error {
	return s.storage.EnsureBucketExists()
}

func (s *Server) ServeTCP(ln net.Listener) error {
	rs := redcon.NewServerNetwork("tcp", ln.Addr().String(), s.handle, s.accept, s.onClosed)
	s.close = rs.Close
	return rs.Serve(ln)
}

func (s *Server) Close() error {
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
	case op.Exists:
		s.exists(conn, args)
	case op.Keys:
		s.keys(conn, args)
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

	items, err := s.storage.GetDB()
	if err != nil {
		writeErr(conn, err)
		return
	}
	val, ok := items[args[0]]
	if !ok {
		conn.WriteNull()
		return
	}
	conn.WriteBulkString(val)
}

func (s *Server) set(conn redcon.Conn, args []string) {
	if len(args) != 2 {
		writeErrArity(conn, op.Set)
		return
	}

	_, err := s.storage.MutateDB(func(items map[string]string) (int, error) {
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
	if len(args) == 0 {
		writeErrArity(conn, op.Del)
		return
	}

	n, err := s.storage.MutateDB(func(items map[string]string) (int, error) {
		var n int
		for _, k := range args {
			_, ok := items[k]
			if ok {
				n++
			}
			delete(items, k)
		}
		return n, nil
	})

	if err != nil {
		writeErr(conn, err)
		return
	}
	conn.WriteInt(n)
}

func (s *Server) exists(conn redcon.Conn, args []string) {
	if len(args) == 0 {
		writeErrArity(conn, op.Exists)
		return
	}

	items, err := s.storage.GetDB()
	if err != nil {
		writeErr(conn, err)
		return
	}

	var n int
	for _, k := range args {
		_, ok := items[k]
		if ok {
			n++
		}
	}
	conn.WriteInt(n)
}

func (s *Server) keys(conn redcon.Conn, args []string) {
	if len(args) != 1 {
		writeErrArity(conn, op.Keys)
		return
	}
	if args[0] != "*" {
		conn.WriteError("ERR only supported glob is '*'")
		return
	}

	items, err := s.storage.GetDB()
	if err != nil {
		writeErr(conn, err)
		return
	}

	keys := slices.Collect(maps.Keys(items))
	conn.WriteArray(len(keys))
	for _, k := range keys {
		conn.WriteBulkString(k)
	}
}

func (s *Server) flushAll(conn redcon.Conn, args []string) {
	if len(args) > 0 {
		writeErrArity(conn, op.FlushAll)
		return
	}

	_, err := s.storage.MutateDB(func(items map[string]string) (int, error) {
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
