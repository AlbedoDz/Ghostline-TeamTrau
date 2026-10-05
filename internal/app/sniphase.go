package app

import (
	"context"
	"errors"
	"slices"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/store"
)

// FakeSNIWarningVersion is the version of the mandatory Fake SNI warning;
// bump it when its content changes so users read it again.
const FakeSNIWarningVersion = 1

const reasonFakeSNI = "fakesni"

// sniState is what the Fake SNI phase changed; guarded by opMu.
type sniState struct {
	issuer    *certs.Issuer
	installed []string // session CA thumbprints in the Root store
	domains   []string
}

func (o *Orchestrator) sniDomains() []string {
	if o.d.Rules == nil {
		return nil
	}
	if c := o.d.Rules(); c != nil {
		return c.SNIDomains()
	}
	return nil
}

// startSNIPhase runs S0–S4 (spec 2B 6.2) after the proxy and DNS server
// phases. A failure undoes the S steps and marks the connection degraded;
// DNS is never touched. Callers hold opMu.
func (o *Orchestrator) startSNIPhase(ctx context.Context) error {
	fs := o.d.Settings().FakeSNI
	domains := o.sniDomains()
	if o.d.Certs == nil || o.d.SetMITM == nil || !fs.Enabled || fs.AckVersion < FakeSNIWarningVersion || len(domains) == 0 {
		o.update(func(sn *Snapshot) { sn.FakeSNI = FakeSNIStatus{} })
		o.clearReason(reasonFakeSNI)
		return nil
	}
	if !o.px.running {
		o.update(func(sn *Snapshot) {
			sn.FakeSNI = FakeSNIStatus{NeedsProxy: true, Error: &AppError{Code: CodeFakeSNINeedsProxy}}
		})
		o.clearReason(reasonFakeSNI)
		return nil
	}
	var ca *certs.CA
	steps := []step{
		{name: "sni.ca", do: func(context.Context) error {
			var err error
			if ca, err = certs.NewSessionCA(domains, o.d.Now()); err != nil {
				if errors.Is(err, certs.ErrTooManyDomains) {
					return appErr(CodeFakeSNITooMany, err, "count", len(domains))
				}
				return appErr(CodeCertInstallFailed, err, "kind", "session")
			}
			return nil
		}},
		{name: "sni.state", do: func(context.Context) error {
			// Write-ahead: the thumbprint is recorded before the CA exists
			// in Root, so every recovery layer can remove it.
			if err := ignoreNoChange(o.setState(func(st *store.State) { st.AddSessionCert(ca.Thumbprint()) })); err != nil {
				return appErr(CodeCertInstallFailed, err, "kind", "session")
			}
			return nil
		}, undo: func(context.Context) error {
			if slices.Contains(o.sni.installed, ca.Thumbprint()) {
				return nil // still in Root: keep it recorded
			}
			return ignoreNoChange(o.setState(func(st *store.State) { st.RemoveSessionCert(ca.Thumbprint()) }))
		}},
		{name: "sni.install", do: func(context.Context) error {
			if err := o.d.Certs.InstallSession(ca.DER); err != nil {
				return appErr(CodeCertInstallFailed, err, "kind", "session")
			}
			o.sni.installed = append(o.sni.installed, ca.Thumbprint())
			return nil
		}, undo: func(context.Context) error {
			return o.removeSessionCA(ca.Thumbprint())
		}},
		{name: "sni.activate", do: func(ctx context.Context) error {
			o.sni.issuer = certs.NewIssuer(ca, o.d.Now)
			o.d.SetMITM(o.sni.issuer)
			if o.d.MITMSelfTest != nil {
				if err := o.d.MITMSelfTest(ctx); err != nil {
					// runSteps undoes only earlier steps: clear this one here.
					o.d.SetMITM(nil)
					o.sni.issuer = nil
					return appErr(CodeFakeSNISelfTest, err)
				}
			}
			return nil
		}, undo: func(context.Context) error {
			o.d.SetMITM(nil)
			o.sni.issuer = nil
			return nil
		}},
	}
	if err := runSteps(ctx, steps, func(int) {}); err != nil {
		var ae *AppError
		if !errors.As(err, &ae) {
			ae = appErr(CodeCertInstallFailed, err, "kind", "session")
		}
		o.update(func(sn *Snapshot) { sn.FakeSNI = FakeSNIStatus{Error: &AppError{Code: ae.Code, Params: ae.Params}} })
		o.addReason(reasonFakeSNI)
		o.log("fakesni", ae.Code, flatten(ae.Params)...)
		return err
	}
	o.sni.domains = domains
	o.setSNIStatus()
	o.clearReason(reasonFakeSNI)
	o.log("fakesni", "FAKESNI_STARTED", "domains", len(domains))
	return nil
}

func (o *Orchestrator) setSNIStatus() {
	ca := o.sni.issuer.CA()
	o.update(func(sn *Snapshot) {
		sn.FakeSNI = FakeSNIStatus{Active: true, Domains: len(o.sni.domains), Thumbprint: ca.Thumbprint(), NotAfter: ca.Cert.NotAfter}
	})
}

// removeSessionCA removes one session CA from Root and, only then, from
// state.json. A failure keeps it recorded for recovery and warns.
func (o *Orchestrator) removeSessionCA(thumb string) error {
	if err := o.d.Certs.RemoveSession(thumb); err != nil {
		o.AddWarning(AppError{Code: CodeCertRemoveFailed, Params: map[string]any{"thumbprint": thumb}})
		o.log("fakesni", CodeCertRemoveFailed, "thumbprint", thumb)
		return err
	}
	o.sni.installed = slices.DeleteFunc(o.sni.installed, func(x string) bool { return x == thumb })
	return ignoreNoChange(o.setState(func(st *store.State) { st.RemoveSessionCert(thumb) }))
}

// stopSNIPhase turns Fake SNI off and removes every session CA it
// installed. Callers hold opMu.
func (o *Orchestrator) stopSNIPhase(context.Context) {
	if o.d.SetMITM != nil && o.sni.issuer != nil {
		o.d.SetMITM(nil)
	}
	o.sni.issuer, o.sni.domains = nil, nil
	for _, t := range slices.Clone(o.sni.installed) {
		_ = o.removeSessionCA(t)
	}
	o.update(func(sn *Snapshot) { sn.FakeSNI = FakeSNIStatus{} })
	o.clearReason(reasonFakeSNI)
}

// ReapplyFakeSNI restarts the Fake SNI phase after its settings changed.
// It does nothing while not connected.
func (o *Orchestrator) ReapplyFakeSNI(ctx context.Context) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return nil
	}
	o.stopSNIPhase(ctx)
	return o.startSNIPhase(ctx)
}
