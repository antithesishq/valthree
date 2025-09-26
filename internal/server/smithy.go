package server

import (
	"errors"

	"github.com/aws/smithy-go"
)

func hasSmithyCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var smithyErr smithy.APIError
	if errors.As(err, &smithyErr) && smithyErr.ErrorCode() == code {
		return true
	}
	return false
}
