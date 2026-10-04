package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckUpdateNow_Delegates(t *testing.T) {
	sh := newSvc(t)
	_, err := sh.svc.CheckUpdateNow()
	require.Error(t, err) // not wired

	sh.svc.x.CheckUpdate = func(ctx context.Context) (UpdateCheck, error) {
		_, ok := ctx.Deadline()
		require.True(t, ok, "manual check must have a timeout")
		return UpdateCheck{Current: "0.2.1", Latest: "v0.2.2", URL: "u", Newer: true}, nil
	}
	r, err := sh.svc.CheckUpdateNow()
	require.NoError(t, err)
	require.True(t, r.Newer)

	sh.svc.x.CheckUpdate = func(context.Context) (UpdateCheck, error) { return UpdateCheck{}, errors.New("offline") }
	_, err = sh.svc.CheckUpdateNow()
	require.ErrorContains(t, err, CodeUpdateCheckFailed)
}
