package app

import (
	"context"
	"errors"
	"log/slog"
)

// step is one reversible part of the connect saga.
type step struct {
	name string
	do   func(context.Context) error
	undo func(context.Context) error
}

// errHalt marks a failure after which rolling back further would be unsafe:
// DNS could not be restored, so the engine, watchdog, recovery task and the
// dns_set state must all stay in place for later safety layers.
var errHalt = errors.New("rollback halted: DNS not restored")

type haltError struct{ ae *AppError }

func (h *haltError) Error() string        { return h.ae.Error() }
func (h *haltError) Is(target error) bool { return target == errHalt }
func (h *haltError) Unwrap() error        { return h.ae }

func halt(ae *AppError) error { return &haltError{ae: ae} }

// runSteps runs steps in order. When step i fails, the undo of steps i-1…0
// runs in reverse and the original error is returned. A step or undo that
// returns a halt error stops the rollback right there and that halt error is
// returned. onStep is told the 1-based index before each step.
func runSteps(ctx context.Context, steps []step, onStep func(int)) error {
	for i, s := range steps {
		onStep(i + 1)
		err := ctx.Err()
		if err == nil {
			err = s.do(ctx)
		}
		if err == nil {
			continue
		}
		if errors.Is(err, errHalt) {
			return err
		}
		// Undo with a fresh context: cancellation must not stop cleanup.
		uctx := context.WithoutCancel(ctx)
		for j := i - 1; j >= 0; j-- {
			if steps[j].undo == nil {
				continue
			}
			if uerr := steps[j].undo(uctx); uerr != nil {
				if errors.Is(uerr, errHalt) {
					slog.Warn("rollback halted", "step", steps[j].name, "cause", err)
					return uerr
				}
				slog.Warn("undo failed", "step", steps[j].name, "err", uerr)
			}
		}
		return err
	}
	return nil
}
