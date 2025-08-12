package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

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
	_, err := s.client.CreateBucket(context.TODO(), &s3.CreateBucketInput{
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

		n, err := f(items)
		if err != nil {
			return 0, err
		}

		err = s.setDB(items, etag)
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
			return make(map[string]string), "", nil
		}
		return nil, "", fmt.Errorf("get object: %v", err)
	}
	defer res.Body.Close()
	if res.ETag == nil || *res.ETag == "" {
		return nil, "", errors.New("response has no etag")
	}
	items := make(map[string]string)
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		return nil, "", fmt.Errorf("unmarshal: %v", err)
	}
	return items, *res.ETag, nil
}

func (s *storage) setDB(items map[string]string, etag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	bs, err := json.Marshal(items)
	if err != nil {
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
		if errors.As(err, &apiErr) {
			fmt.Println(err.Error()) // FIXME, don't know correct code
			return errMismatchedETag
		}
		return fmt.Errorf("put object: %v", err)
	}
	return nil
}
