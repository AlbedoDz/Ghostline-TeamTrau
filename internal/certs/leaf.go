package certs

import (
	"container/list"
	"crypto/tls"
	"sync"
	"time"
)

const (
	leafCacheSize = 1000
	leafLife      = 7 * 24 * time.Hour
)

// Issuer signs and caches per-host certificates for Fake SNI. It satisfies
// mitm.LeafSource.
type Issuer struct {
	ca  *CA
	now func() time.Time

	mu    sync.Mutex
	order *list.List // front = most recent; values are host strings
	byKey map[string]*list.Element
	certs map[string]*tls.Certificate
}

// NewIssuer creates an issuer for ca.
func NewIssuer(ca *CA, now func() time.Time) *Issuer {
	return &Issuer{ca: ca, now: now, order: list.New(), byKey: map[string]*list.Element{}, certs: map[string]*tls.Certificate{}}
}

// CA returns the issuing CA.
func (i *Issuer) CA() *CA { return i.ca }

// Leaf returns a certificate for host, signing one when it is not cached
// or is about to expire.
func (i *Issuer) Leaf(host string) (*tls.Certificate, error) {
	now := i.now()
	i.mu.Lock()
	if c, ok := i.certs[host]; ok && now.Add(time.Hour).Before(c.Leaf.NotAfter) {
		i.order.MoveToFront(i.byKey[host])
		i.mu.Unlock()
		return c, nil
	}
	i.mu.Unlock()

	c, err := i.ca.IssueServer([]string{host}, nil, leafLife, now)
	if err != nil {
		return nil, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if e, ok := i.byKey[host]; ok {
		i.order.MoveToFront(e)
	} else {
		i.byKey[host] = i.order.PushFront(host)
	}
	i.certs[host] = c
	for i.order.Len() > leafCacheSize {
		old := i.order.Back()
		h := old.Value.(string)
		i.order.Remove(old)
		delete(i.byKey, h)
		delete(i.certs, h)
	}
	return c, nil
}
