package lists

import (
	"context"
	"errors"
	"os"

	"github.com/hashcott/ghostline/internal/servers"
)

// CodeSignatureInvalid is the UI code for a signed list whose signature is
// missing or wrong.
const CodeSignatureInvalid = "LIST_SIGNATURE_INVALID"

// ErrSignatureInvalid means a signed list failed verification; the cached
// copy (if any) is kept.
var ErrSignatureInvalid = errors.New(CodeSignatureInvalid)

func (f *Fetcher) sigPath(id string) string { return f.cachePath(id, "") + ".sig" }

// verify checks data against sig with the fetcher's key. A missing key
// fails: a signed list is never accepted unchecked.
func (f *Fetcher) verify(data, sig []byte) error {
	if len(f.SigKey) == 0 || len(sig) == 0 || servers.VerifySigned(data, sig, f.SigKey) != nil {
		return ErrSignatureInvalid
	}
	return nil
}

// fetchSig downloads the signature next to the list URL that served data.
func (f *Fetcher) fetchSig(ctx context.Context, used string) ([]byte, error) {
	sig, _, err := f.get(ctx, used+".sig", "", "")
	if err != nil {
		return nil, ErrSignatureInvalid
	}
	return sig, nil
}

// verifyCached re-checks the cached copy of a signed list.
func (f *Fetcher) verifyCached(id string, data []byte) error {
	sig, err := os.ReadFile(f.sigPath(id))
	if err != nil {
		return ErrSignatureInvalid
	}
	return f.verify(data, sig)
}
