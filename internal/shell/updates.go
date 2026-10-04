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

// set records a newer release and reports whether it was not known yet.
func (u *updateState) set(tag, url string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.tag == tag && u.url == url {
		return false
	}
	u.tag, u.url = tag, url
	return true
}

// releaseInterval is how often a running app asks for the latest release.
const releaseInterval = 6 * time.Hour

// releaseCheck asks for the latest release at every app start, every
// releaseInterval while running, and whenever no release is remembered yet
// (meta written by v0.1.0/0.1.1 had a recent lastUpdateCheck but no tag, so
// waiting for the interval hid new releases for up to a day). The result is
// remembered in meta so the notice survives restarts and failed checks. ok
// is false when the running version is already current.
func releaseCheck(meta *store.Meta, now time.Time, current string, startup bool, latest func() (updater.Release, error)) (updater.Release, bool) {
	if startup || meta.LatestTag == "" || now.Sub(meta.LastUpdateCheck) >= releaseInterval {
		if r, err := latest(); err == nil {
			meta.LastUpdateCheck = now
			meta.LatestTag, meta.LatestURL = r.Tag, r.URL
		}
	}
	if meta.LatestTag == "" || !updater.Newer(current, meta.LatestTag) {
		return updater.Release{}, false
	}
	return updater.Release{Tag: meta.LatestTag, URL: meta.LatestURL}, true
}

// runUpdates performs the once-a-day jobs: release check, signed server
// list and the DNSCrypt resolver list. Failures are only logged. onUpdate
// (may be nil) is told about a newer release once per tag.
func runUpdates(ctx context.Context, paths store.Paths, box *app.SettingsBox, cat *catalog, st *updateState, bus *app.Bus, log *slog.Logger, onUpdate func(tag, url string)) {
	client := &http.Client{Timeout: 30 * time.Second}
	tick := time.NewTicker(releaseInterval)
	defer tick.Stop()
	startup := true
	for {
		meta := store.LoadMeta(paths.Meta)
		now := time.Now()
		s := box.Get()
		if s.Updates.CheckApp {
			r, ok := releaseCheck(&meta, now, brand.Version, startup, func() (updater.Release, error) {
				r, err := updater.Latest(ctx, client, brand.ReleasesAPI)
				if err != nil {
					log.Info("update check", "code", app.CodeUpdateCheckFailed, "err", err)
				}
				return r, err
			})
			if ok && st.set(r.Tag, r.URL) {
				bus.Emit(app.EventUpdate, app.UpdateInfo{Tag: r.Tag, URL: r.URL})
				if onUpdate != nil {
					onUpdate(r.Tag, r.URL)
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
		startup = false
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
