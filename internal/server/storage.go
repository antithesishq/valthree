package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

var errMismatchedETag = fmt.Errorf("mismatched ETags")

type storage struct {
	timeout time.Duration
	bucket  string
	name    string

	mu     sync.Mutex // serializing ops reduces retries
	client *s3.Client
}

func (s *storage) EnsureBucketExists() error {
	_, err := s.client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "BucketAlreadyOwnedByYou" {
			return nil
		}
	}
	return err
}

func (s *storage) MutateDB(f func(map[string]string) (int, error)) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		items, etag, err := s.getDB()
		if err != nil {
			return 0, err
		}

		// Create a copy of the items map to avoid mutating the original
		// This ensures that if we need to retry due to ETag mismatch,
		// we start with fresh data from the database
		itemsCopy := make(map[string]string, len(items))
		for k, v := range items {
			itemsCopy[k] = v
		}

		n, err := f(itemsCopy)
		if err != nil {
			return 0, err
		}

		err = s.setDB(itemsCopy, etag)
		if err != nil && !errors.Is(err, errMismatchedETag) {
			return 0, err
		} else if err == nil {
			return n, nil
		}
	}
}

func (s *storage) GetDB() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, _, err := s.getDB()
	return items, err
}

func (s *storage) getDB() (map[string]string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	res, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.name),
	})
	if err != nil {
		var errNoKey *types.NoSuchKey
		if errors.As(err, &errNoKey) {
			// The client has issued a GET or DEL before any SET succeeds, so there's
			// no database file in object storage. The server should treat this just
			// like a GET or DEL of a key that doesn't yet exist.
			//
			// If our random workload hasn't exercised this logic, it's not thorough
			// enough and we should fail the Antithesis run.
			assert.Reachable("Exercised GET or DEL before database creation", nil)
			return make(map[string]string), "", nil
		}
		// Adequate fault injection would make reads from object storage fail
		// sometimes, even if the object exists.
		assert.Reachable("Exercised failures reading from object storage", nil)
		return nil, "", fmt.Errorf("get object: %v", err)
	}
	defer res.Body.Close()
	if res.ETag == nil || *res.ETag == "" {
		// With our client configuration, we believe that this branch is
		// unreachable. If Antithesis can force us into this branch, fail the run.
		assert.Unreachable("Database always has an ETag", nil)
		return nil, "", errors.New("response has no etag")
	}
	items := make(map[string]string)
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		// If we reach this branch, the write path is broken - we should never have
		// invalid JSON in object storage.
		assert.Unreachable("Database in object storage is always valid JSON", nil)
		return nil, "", fmt.Errorf("unmarshal: %v", err)
	}
	return items, *res.ETag, nil
}

func (s *storage) setDB(items map[string]string, etag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	bs, err := json.Marshal(items)
	if err != nil {
		// Our tests and workloads only send valid UTF-8, so this should be
		// unreachable.
		assert.Unreachable("Database in memory is always valid JSON", nil)
		return fmt.Errorf("marshal JSON: %v", err)
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.name),
		Body:   bytes.NewReader(bs),
	}
	if etag == "" {
		input.IfNoneMatch = aws.String("*")
	} else {
		input.IfMatch = aws.String(etag)
	}

	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
			// This is the most critical code in the Valthree server: it ensures that
			// writes are serialized, even when clients connect to different Valthree
			// servers. It's critical that Antithesis exercise this code path.
			assert.Reachable("Exercised optimistic concurrency control rollback", nil)
			return errMismatchedETag
		}
		// Of course, we should also exercise other errors in the write path.
		assert.Reachable("Exercised failures writing to object storage", nil)
		return fmt.Errorf("put object: %v", err)
	}
	return nil
}
