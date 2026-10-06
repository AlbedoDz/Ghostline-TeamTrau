// Command fetchdnscrypt refreshes the DNSCrypt resolver list embedded in
// the app (lists/dnscrypt), so a fresh install has every server before its
// first download. Run before each release; the signature is checked here
// and again by the app.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/updater"
)

func main() {
	dir := flag.String("out", "lists/dnscrypt", "output directory")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	md, sig, err := updater.FetchDNSCrypt(ctx, &http.Client{Timeout: 30 * time.Second}, brand.DNSCryptListURLs, brand.DNSCryptMinisignKey)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*dir, "public-resolvers.md"), md, 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*dir, "public-resolvers.md.minisig"), sig, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d bytes and the signature to %s\n", len(md), *dir)
}
