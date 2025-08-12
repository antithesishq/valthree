// Package client provides a more convenient wrapper around the redigo client.
package client

import (
	"fmt"
	"net"

	"github.com/gomodule/redigo/redis"
)

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

// Del deletes one or more keys. It returns an error if the server reports fewer
// keys deleted than requested.
func (c *Client) Del(keys ...string) error {
	if c.connErr != nil {
		return fmt.Errorf("conn unusable: %w", c.connErr)
	}
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	res, err := c.conn.Do("DEL", args...)
	if err != nil {
		return err
	}
	r, ok := res.(int64)
	if !ok {
		return fmt.Errorf("unexpected del response type: %T", res)
	}
	if r != int64(len(keys)) {
		return fmt.Errorf("unexpected del response: %d", r)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return fmt.Errorf("conn unusable: %w", err)
	}
	return nil
}

// Exists checks if all the supplied keys exist.
func (c *Client) Exists(keys ...string) (bool, error) {
	if c.connErr != nil {
		return false, fmt.Errorf("conn unusable: %w", c.connErr)
	}
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	res, err := c.conn.Do("EXISTS", args...)
	if err != nil {
		return false, err
	}
	r, ok := res.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected exists response type: %T", res)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return false, fmt.Errorf("conn unusable: %w", err)
	}
	return r == int64(len(keys)), nil
}

// Keys returns all keys in the database. Under the hood, it sends "KEYS *" to
// the database.
func (c *Client) Keys() ([]string, error) {
	if c.connErr != nil {
		return nil, fmt.Errorf("conn unusable: %w", c.connErr)
	}
	res, err := c.conn.Do("KEYS", "*")
	if err != nil {
		return nil, err
	}
	r, ok := res.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected keys response type: %T", res)
	}
	keys := make([]string, len(r))
	for i, key := range r {
		k, ok := key.([]byte)
		if !ok {
			return nil, fmt.Errorf("unexpected keys response element type: %T", key)
		}
		keys[i] = string(k)
	}
	if err := c.conn.Err(); err != nil {
		c.connErr = err
		_ = c.conn.Close()
		return nil, fmt.Errorf("conn unusable: %w", err)
	}
	return keys, nil
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
