package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/minio"
	"go.akshayshah.org/attest"
)

func TestConditionalWrite(t *testing.T) {
	type DB = map[string]string

	s := newStorage(t)

	// Initial state is an empty DB without an ETag.
	db, etag, err := s.getDB()
	attest.Ok(t, err)
	attest.Equal(t, db, DB{})
	attest.Zero(t, etag)

	// Saving the DB alters the ETag.
	err = s.setDB(DB{"foo": "bar"}, etag)
	attest.Ok(t, err)
	db, etag, err = s.getDB()
	attest.Ok(t, err)
	attest.Equal(t, db, DB{"foo": "bar"})
	attest.NotZero(t, etag)

	// There's a DB saved, so writes must pass the current ETag.
	for _, wrong := range []string{"", "not-the-right-ETag"} {
		err := s.setDB(DB{"baz": "quux"}, wrong)
		attest.ErrorIs(t, err, errMismatchedETag)
	}

	// With the right ETag, we can overwrite the DB.
	previousETag := etag
	err = s.setDB(DB{"baz": "quux"}, etag)
	attest.Ok(t, err)
	db, etag, err = s.getDB()
	attest.Ok(t, err)
	attest.Equal(t, db, DB{"baz": "quux"})
	attest.NotEqual(t, etag, previousETag)
}

func newStorage(tb testing.TB) *storage {
	tb.Helper()
	const user, password = "admin", "password"
	// The MinIO testcontainers module includes verbose test logs by default.
	mc, err := minio.Run(
		tb.Context(),
		"minio/minio:RELEASE.2025-07-23T15-54-02Z",
		minio.WithUsername(user),
		minio.WithPassword(password),
	)
	attest.Ok(tb, err, attest.Sprint("start MinIO container"))
	addr, err := mc.ConnectionString(tb.Context())
	attest.Ok(tb, err, attest.Sprint("get MinIO conn str"))

	// We could refactor server construction to isolate S3 client creation, but
	// it's simpler to just yank off the unexported storage struct.
	srv := New(Config{
		DatabaseName: "test",
		MaxItems:     1024,
		S3Endpoint:   fmt.Sprintf("http://%s", addr),
		S3Region:     "us-east-1",
		S3User:       user,
		S3Password:   password,
		S3Bucket:     "valthree",
		S3Timeout:    time.Second,
	})
	attest.Ok(tb, srv.EnsureBucketExists(), attest.Sprint("create bucket"))
	return &srv.storage
}
