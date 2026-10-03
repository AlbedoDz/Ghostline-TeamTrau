package app

import (
	"context"
	"log/slog"
)

// step is one reversible part of the connect saga.
type step struct {
	name string
	do   func(context.Context) error
	undo func(context.Context) error
}

// runSteps runs steps in order. When step i fails, the undo of steps i-1…0
// runs in reverse (undo errors are logged, not returned) and the original
// error is returned. onStep is told the 1-based index before each step.
func runSteps(ctx context.Context, steps []step, onStep func(int)) error {
	for i, s := range steps {
		onStep(i + 1)
		err := ctx.Err()
		if err == nil {
			err = s.do(ctx)
		}
		if err != nil {
			// Undo with a fresh context: cancellation must not stop cleanup.
			uctx := context.WithoutCancel(ctx)
			for j := i - 1; j >= 0; j-- {
				if steps[j].undo == nil {
					continue
				}
				if uerr := steps[j].undo(uctx); uerr != nil {
					slog.Warn("undo failed", "step", steps[j].name, "err", uerr)
				}
			}
			return err
		}
	}
	return nil
}
