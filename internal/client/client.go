// Package client provides a more convenient wrapper around a redigo connection.
package client

import (
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/gomodule/redigo/redis"
)

// ErrNotFound signals that the key used in a GET or DEL command was
// not present in the database.
var ErrNotFound = errors.New("key not found")

// Client is a type-safe, lower-boilerplate wrapper around the redigo client. It
// doesn't have all the flexibility of a plain redigo connection, but it
// introduces less noise in tests.
//
// Clients are not safe for concurrent use.
type Client struct {
	conn    redis.Conn
	connErr error
}

// New creates a new Client.
func New(addr net.Addr) (*Client, error) {
	conn, err := redis.Dial("tcp", addr.String())
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	return &Client{conn: conn}, nil
}

// Ping the database.
func (c *Client) Ping() error {
	if c.connErr != nil {
		return fmt.Errorf("conn unusable: %w", c.connErr)
	}
	res, err := c.conn.Do("PING")
	if err != nil {
		return err
	}
	r, ok := res.(string)
	if !ok {
		return fmt.Errorf("unexpected ping response type: %T", res)
	}
	if r != "PONG" {
		return fmt.Errorf("unexpected ping response: %s", r)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return fmt.Errorf("conn unusable: %w", err)
	}
	return nil
}

// Get the value of a single key.
func (c *Client) Get(key string) (string, error) {
	if c.connErr != nil {
		return "", fmt.Errorf("conn unusable: %w", c.connErr)
	}
	res, err := c.conn.Do("GET", key)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", ErrNotFound
	}
	r, ok := res.([]byte)
	if !ok {
		return "", fmt.Errorf("unexpected get response type: %T", res)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return "", fmt.Errorf("conn unusable: %w", err)
	}
	return string(r), nil
}

// Set the value of a single key.
func (c *Client) Set(key, value string) error {
	if c.connErr != nil {
		return fmt.Errorf("conn unusable: %w", c.connErr)
	}
	res, err := c.conn.Do("SET", key, value)
	if err != nil {
		return err
	}
	r, ok := res.(string)
	if !ok {
		return fmt.Errorf("unexpected set response type: %T", res)
	}
	if r != "OK" {
		return fmt.Errorf("unexpected set response: %s", r)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return fmt.Errorf("conn unusable: %w", err)
	}
	return nil
}

// Del deletes a key.
func (c *Client) Del(key string) error {
	if c.connErr != nil {
		return fmt.Errorf("conn unusable: %w", c.connErr)
	}
	res, err := c.conn.Do("DEL", key)
	if err != nil {
		return err
	}
	r, ok := res.(int64)
	if !ok {
		return fmt.Errorf("unexpected del response type: %T", res)
	}
	if r > 1 {
		return fmt.Errorf("server returned %d for single-key DEL", r)
	}
	if r == 0 {
		return ErrNotFound
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return fmt.Errorf("conn unusable: %w", err)
	}
	return nil
}

// Count returns the number of keys in the database.
func (c *Client) Count() (int64, error) {
	if c.connErr != nil {
		return 0, fmt.Errorf("conn unusable: %w", c.connErr)
	}
	res, err := c.conn.Do("COUNT")
	if err != nil {
		return 0, err
	}
	r, ok := res.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected count response type: %T", res)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return 0, fmt.Errorf("conn unusable: %w", err)
	}
	return r, nil
}

// FlushAll deletes all keys in the database.
func (c *Client) FlushAll() error {
	if c.connErr != nil {
		return fmt.Errorf("conn unusable: %w", c.connErr)
	}
	res, err := c.conn.Do("FLUSHALL")
	if err != nil {
		return err
	}
	r, ok := res.(string)
	if !ok {
		return fmt.Errorf("unexpected flushall response type: %T", res)
	}
	if r != "OK" {
		return fmt.Errorf("unexpected flushall response: %s", r)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return fmt.Errorf("conn unusable: %w", err)
	}
	return nil
}

// Close the underlying connection.
func (c *Client) Close() error {
	if c.connErr != nil {
		return fmt.Errorf("conn unusable: %w", c.connErr)
	}
	if err := c.conn.Close(); err != nil {
		c.connErr = err
		return err
	}
	return nil
}

// Close the underlying connection and log any errors.
func (c *Client) CloseAndLog(logger *slog.Logger) {
	if err := c.Close(); err != nil {
		logger.Error("close client", "err", err)
	}
}
