// Package proptest provides utilities for writing property-based tests for
// Valthree servers.
package proptest

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/anishathalye/porcupine"
	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/antithesishq/valthree/internal/client"
	"github.com/antithesishq/valthree/internal/op"
)

// Error is sometimes returned from CheckWorkloads, indicating that
// verification timed out or that the observed behavior includes consistency
// violations.
//
// If the Error indicates a consistency violation, Visualization will be an
// interactive, self-contained HTML document demonstrating the violation.
type Error struct {
	Key           string
	TimedOut      bool
	Visualization *bytes.Buffer
}

// Error implements error.
func (e *Error) Error() string {
	if e.TimedOut {
		return fmt.Sprintf("%s: model timed out", e.Key)
	}
	return fmt.Sprintf("%s: history not linearizable", e.Key)
}

// Arguments for calling a client; used in the porcupine model below.
type args struct {
	Op    op.Op
	Key   string
	Value string
}

// Results from calling a client; used in the porcupine model below.
type rets struct {
	Value string
	Err   error
}

// GenWorkloads generates a workload for a variable number of clients.
func GenWorkloads(r *rand.Rand) [][]porcupine.Operation {
	// To trigger consistency bugs, we want multiple clients to operate
	// concurrently on a handful of keys.
	keys := make([]string, r.IntN(3)+2) // 2-4 keys
	for i := range keys {
		keys[i] = fmt.Sprintf("key%d", i)
	}
	numClientsPerKey := r.IntN(3) + 2 // 2-4 clients per key
	opsPerClient := r.IntN(128) + 128 // 128-255 operations per client
	workloads := make([][]porcupine.Operation, len(keys)*numClientsPerKey)
	// Bias the workload towards GETs, which makes checking for linearizability
	// faster.
	ops := []op.Op{
		op.Get,
		op.Get,
		op.Get,
		op.Set,
		op.Set,
		op.Del,
	}
	for clientId := range workloads {
		key := keys[clientId%len(keys)]
		workload := make([]porcupine.Operation, opsPerClient)
		for i := range workload {
			workload[i] = porcupine.Operation{
				ClientId: clientId,
				Input: &args{
					Op:    ops[r.IntN(len(ops))],
					Key:   key,
					Value: genString(r),
				},
				Output: &rets{},
			}
		}
		workloads[clientId] = workload
	}
	return workloads
}

// RunWorkload runs a workload on a client.
func RunWorkload(logger *slog.Logger, client *client.Client, workload []porcupine.Operation) {
	for i := range workload {
		if i%100 == 0 {
			logger.Debug("running workload", "ops_complete", i, "ops_left", len(workload)-i)
		}
		in := workload[i].Input.(*args)
		out := workload[i].Output.(*rets)
		workload[i].Call = time.Now().UnixNano()
		switch in.Op {
		case op.Get:
			out.Value, out.Err = client.Get(in.Key)
		case op.Set:
			out.Err = client.Set(in.Key, in.Value)
		case op.Del:
			out.Err = client.Del(in.Key)
		default:
			assert.Unreachable("Unexpected operation in workload run", map[string]any{"op": in.Op})
		}
		workload[i].Return = time.Now().UnixNano()
	}
}

// CheckWorkloads verifies that the real-world behavior of the Valthree server,
// as seen by RunWorkload, satisfies strong serializable consistency. When no
// consistency anomalies are found, CheckWorkloads also returns the percentage
// of operations that succeeded (as a measure of liveness).
//
// Verification is NP-hard, so it may time out. If verification fails or times
// out, the returned error will be an *Error.
func CheckWorkloads(deadline time.Duration, workloads [][]porcupine.Operation) (float64, error) {
	// Valthree keys are linearizable. If we've broken something, it's painful to
	// debug the whole workload. Instead, partition the execution history by key
	// and check each partition individually. (Porcupine supports this via
	// Model.Partition, but we have to do it ourselves if we also want to
	// restrict the visualization to a single key.)
	partitioned := make(map[string][]porcupine.Operation)
	var successes, total float64
	for _, history := range workloads {
		for _, op := range history {
			total++
			if op.Output.(*rets).Err == nil {
				successes++
			}
			in := op.Input.(*args)
			partitioned[in.Key] = append(partitioned[in.Key], op)
		}
	}
	progress := successes / total

	for key, history := range partitioned {
		model := newModel()
		cr, info := porcupine.CheckOperationsVerbose(model, history, deadline)
		if cr == porcupine.Ok {
			continue
		}
		if cr == porcupine.Unknown {
			return 0, &Error{Key: key, TimedOut: true}
		}
		var buf bytes.Buffer
		if err := porcupine.Visualize(model, info, &buf); err != nil {
			return 0, err
		}
		return 0, &Error{Key: key, Visualization: &buf}
	}
	return progress, nil
}

func newModel() porcupine.Model {
	return porcupine.Model{
		Init: func() any { return newSet() },
		Step: func(state, input, output any) (bool, any) {
			in := input.(*args)
			out := output.(*rets)
			db := state.(*set)
			switch in.Op {
			case op.Get:
				if out.Err != nil {
					if errors.Is(out.Err, client.ErrNotFound) {
						// Missing keys may be represented by an empty set or a set
						// containing the empty string.
						return db.Contains("") || db.Len() == 0, db
					}
					// Other failures are always okay
					return true, db
				}
				return db.Contains(out.Value), db
			case op.Set:
				if out.Err != nil {
					// Write may have succeeded, so we expand the set of valid values.
					return true, db.With(in.Value)
				}
				// Write definitely succeeded, so there's only one valid value.
				return true, newSet(in.Value)
			case op.Del:
				if out.Err != nil {
					// Delete may have succeeded: represent the potential absence
					// of the key with an empty string.
					return true, db.With("")
				}
				// Delete definitely succeeded, so the key must be missing.
				return true, newSet()
			default:
				assert.Unreachable("Unexpected step operation", map[string]any{"op": in.Op})
				return true, db
			}
		},
		DescribeOperation: func(input, output any) string {
			return describe(input.(*args), output.(*rets))
		},
		Equal: func(left, right any) bool {
			if left == nil || right == nil {
				return left == right
			}
			l := left.(*set)
			r := right.(*set)
			return slices.Equal(l.Items(), r.Items())
		},
	}
}

func describe(in *args, out *rets) string {
	result := out.Value
	if result == "" {
		result = "OK"
	}
	if out.Err != nil {
		result = fmt.Sprintf("ERR %v", out.Err)
	}

	switch in.Op {
	case op.Get:
		return fmt.Sprintf("GET %s = %s", in.Key, result)
	case op.Set:
		return fmt.Sprintf("SET %s %s = %s", in.Key, in.Value, result)
	case op.Del:
		return fmt.Sprintf("DEL %s = %s", in.Key, result)
	default:
		assert.Unreachable("Unexpected describe operation", map[string]any{"op": in.Op})
		return fmt.Sprintf("UNKNOWN %v", in.Op)
	}
}
