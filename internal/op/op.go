// Package op provides constants for the supported Valkey operations.
package op

import "strings"

// An Op is a Valkey operation. Only the most commonly-used operatations are
// supported.
type Op string

const (
	Get      Op = "get"
	Set      Op = "set"
	Del      Op = "del"
	FlushAll Op = "flushall"
	Ping     Op = "ping"
	Quit     Op = "quit"
)

// New creates an Op from wire data. It does not validate that the operation is
// supported.
func New(op []byte) Op {
	return Op(strings.ToLower(string(op)))
}
