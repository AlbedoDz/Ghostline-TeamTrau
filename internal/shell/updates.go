package shell

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/updater"
)

type updateState struct {
	mu       sync.Mutex
	tag, url string
}

func (u *updateState) get() (string, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.tag, u.url
}

// runUpdates performs the once-a-day jobs: release check, signed server
// list and the DNSCrypt resolver list. Failures are only logged.
func runUpdates(ctx context.Context, paths store.Paths, box *app.SettingsBox, cat *catalog, st *updateState, bus *app.Bus, log *slog.Logger) {
	client := &http.Client{Timeout: 30 * time.Second}
	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()
	for {
		meta := store.LoadMeta(paths.Meta)
		now := time.Now()
		s := box.Get()
		if s.Updates.CheckApp && updater.Due(meta.LastUpdateCheck, now) {
			if r, err := updater.Latest(ctx, client, brand.ReleasesAPI); err != nil {
				log.Info("update check", "code", app.CodeUpdateCheckFailed, "err", err)
			} else {
				meta.LastUpdateCheck = now
				if updater.Newer(brand.Version, r.Tag) {
					st.mu.Lock()
					st.tag, st.url = r.Tag, r.URL
					st.mu.Unlock()
					bus.Emit(app.EventUpdate, app.UpdateInfo{Tag: r.Tag, URL: r.URL})
				}
			}
		}
		if s.Updates.UpdateServerList && updater.Due(meta.LastServerList, now) {
			if _, raw, sig, err := updater.FetchServerList(ctx, client, brand.ServerListURL, brand.ServerListSigURL, serverListKey()); err != nil {
				code := "SERVERLIST_FETCH_FAILED"
				if errors.Is(err, servers.ErrBadSignature) {
					code = app.CodeServerListBadSig
				}
				log.Info("server list", "code", code, "err", err)
			} else if os.WriteFile(paths.ServersRemote, raw, 0o644) == nil && os.WriteFile(paths.ServersRemoteSig, sig, 0o644) == nil {
				meta.LastServerList = now
				cat.reload()
			}
		}
		if s.Updates.UpdateServerList && updater.Due(meta.LastDNSCrypt, now) {
			if md, sig, err := updater.FetchDNSCrypt(ctx, client, brand.DNSCryptListURLs, brand.DNSCryptMinisignKey); err != nil {
				log.Info("dnscrypt list", "err", err)
			} else if os.WriteFile(paths.ServersDNSCrypt, md, 0o644) == nil && os.WriteFile(paths.ServersDNSCryptSig, sig, 0o644) == nil {
				meta.LastDNSCrypt = now
				cat.reload()
			}
		}
		_ = store.SaveMeta(paths.Meta, meta)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
