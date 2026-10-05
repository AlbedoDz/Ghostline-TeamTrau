// Command genservers builds lists/servers.json from lists/seed.json and
// optionally signs it.
//
//	go run ./tools/genservers -seed lists/seed.json -out lists/servers.json [-sign-env SERVERLIST_SIGNING_KEY]
//	go run ./tools/genservers -genkey
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
)

func main() {
	seedPath := flag.String("seed", "lists/seed.json", "seed list")
	outPath := flag.String("out", "lists/servers.json", "output list")
	signEnv := flag.String("sign-env", "", "env var holding the base64 ed25519 private key; writes <out>.sig")
	genkey := flag.Bool("genkey", false, "print a new signing keypair and exit")
	signOnly := flag.String("sign-file", "", "only sign this file with -sign-env (writes <file>.sig) and exit")
	flag.Parse()

	if *signOnly != "" {
		if err := signFile(*signOnly, os.Getenv(*signEnv)); err != nil {
			log.Fatal(err)
		}
		fmt.Println("signed", *signOnly)
		return
	}

	if *genkey {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("public=%s\nprivate=%s\n", hex.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv))
		return
	}

	var seed []model.Server
	if err := store.ReadJSON(*seedPath, &seed); err != nil {
		log.Fatalf("read seed: %v", err)
	}
	list, err := Generate(seed, resolveVia("1.1.1.1:53"), time.Now().UTC().Truncate(time.Second))
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		log.Fatal(err)
	}
	if *signEnv != "" {
		if err := signFile(*outPath, os.Getenv(*signEnv)); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("wrote %d servers to %s\n", len(list.Servers), *outPath)
}

// signFile writes <path>.sig, signed with a base64 ed25519 private key.
func signFile(path, key string) error {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return fmt.Errorf("the signing key is not a base64 ed25519 private key")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".sig", servers.Sign(data, ed25519.PrivateKey(raw)), 0o644)
}

// resolveVia resolves A and AAAA records through a fixed plain-DNS server.
// This runs at build time only, never inside the app.
func resolveVia(server string) func(string) ([]string, error) {
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, server)
	}}
	return func(host string) ([]string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ips, err := r.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ips))
		for _, ip := range ips {
			out = append(out, ip.IP.String())
		}
		return out, nil
	}
}
