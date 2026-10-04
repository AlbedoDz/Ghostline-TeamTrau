package sysproxy_test

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	cur     store.SysProxySnapshot
	sets    []store.SysProxySnapshot
	swallow bool
	qerr    error
}

func (f *fakeAPI) Query() (store.SysProxySnapshot, error) { return f.cur, f.qerr }
func (f *fakeAPI) Set(s store.SysProxySnapshot) error {
	f.sets = append(f.sets, s)
	if !f.swallow {
		f.cur = s
	}
	return nil
}

func TestApply_SetsAndReadsBack(t *testing.T) {
	api := &fakeAPI{cur: store.SysProxySnapshot{Flags: sysproxy.FlagDirect}}
	m := sysproxy.Manager{API: api}
	require.NoError(t, m.Apply("127.0.0.1:8080"))
	require.Equal(t, store.SysProxySnapshot{Flags: sysproxy.FlagDirect | sysproxy.FlagProxy, Server: "127.0.0.1:8080", Bypass: sysproxy.Bypass}, api.cur)
	ours, err := m.IsOurs("127.0.0.1:8080")
	require.NoError(t, err)
	require.True(t, ours)
}

func TestApply_ReadsBack(t *testing.T) {
	api := &fakeAPI{swallow: true}
	err := sysproxy.Manager{API: api}.Apply("127.0.0.1:8080")
	require.ErrorIs(t, err, sysproxy.ErrNotApplied)
}

func TestExisting(t *testing.T) {
	m := sysproxy.Manager{}
	s, p, has := m.Existing(store.SysProxySnapshot{Flags: sysproxy.FlagDirect | sysproxy.FlagProxy, Server: "10.0.0.1:3128"})
	require.True(t, has)
	require.Equal(t, "10.0.0.1:3128", s)
	require.Empty(t, p)
	_, p, has = m.Existing(store.SysProxySnapshot{Flags: sysproxy.FlagDirect | sysproxy.FlagAutoProxyURL, AutoconfigURL: "http://wpad/x.pac"})
	require.True(t, has)
	require.Equal(t, "http://wpad/x.pac", p)
	_, _, has = m.Existing(store.SysProxySnapshot{Flags: sysproxy.FlagDirect, Server: "10.0.0.1:3128"})
	require.False(t, has) // a server that is not enabled does not count
	_, _, has = m.Existing(store.SysProxySnapshot{Flags: sysproxy.FlagDirect})
	require.False(t, has)
}

func TestRestoreIfOurs(t *testing.T) {
	snap := store.SysProxySnapshot{Flags: sysproxy.FlagDirect | sysproxy.FlagAutoProxyURL, AutoconfigURL: "http://wpad/x.pac"}
	api := &fakeAPI{cur: snap}
	m := sysproxy.Manager{API: api}
	require.NoError(t, m.Apply("127.0.0.1:8080"))
	restored, err := m.RestoreIfOurs("127.0.0.1:8080", snap)
	require.NoError(t, err)
	require.True(t, restored)
	require.Equal(t, snap, api.cur)

	// Another app replaced our setting: leave it alone.
	api = &fakeAPI{cur: store.SysProxySnapshot{Flags: sysproxy.FlagDirect | sysproxy.FlagProxy, Server: "10.0.0.1:3128"}}
	m = sysproxy.Manager{API: api}
	restored, err = m.RestoreIfOurs("127.0.0.1:8080", snap)
	require.NoError(t, err)
	require.False(t, restored)
	require.Empty(t, api.sets)

	// Our server string but proxy turned off by the user: not ours either.
	api = &fakeAPI{cur: store.SysProxySnapshot{Flags: sysproxy.FlagDirect, Server: "127.0.0.1:8080"}}
	restored, err = sysproxy.Manager{API: api}.RestoreIfOurs("127.0.0.1:8080", snap)
	require.NoError(t, err)
	require.False(t, restored)
}

func TestRestoreIfOurs_UnreadableRestores(t *testing.T) {
	snap := store.SysProxySnapshot{Flags: sysproxy.FlagDirect}
	api := &fakeAPI{qerr: errors.New("query failed")}
	restored, err := sysproxy.Manager{API: api}.RestoreIfOurs("127.0.0.1:8080", snap)
	require.NoError(t, err)
	require.True(t, restored)
	require.Equal(t, []store.SysProxySnapshot{snap}, api.sets)
}
