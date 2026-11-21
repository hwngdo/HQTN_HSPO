package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"hspo/release-v198/appupdate"
)

func main() {
	action := flag.String("action", "", "keygen, public, or sign")
	keyPath := flag.String("key", "", "local Ed25519 signing key path")
	version := flag.String("version", "", "QA application release version")
	setup := flag.String("setup", "", "Windows setup executable to sign")
	notes := flag.String("notes", "", "UTF-8 release notes file")
	output := flag.String("out", "", "signed manifest file")
	flag.Parse()
	if err := run(*action, *keyPath, *version, *setup, *notes, *output); err != nil {
		log.Fatal(err)
	}
}

func run(action, keyPath, version, setup, notes, output string) error {
	if action == "keygen" {
		seed := make([]byte, ed25519.SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return err
		}
		defer clear(seed)
		file, err := os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(file, hex.EncodeToString(seed))
		return errors.Join(err, file.Close())
	}
	content, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	defer clear(content)
	seed, err := hex.DecodeString(strings.TrimSpace(string(content)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("invalid signing key")
	}
	defer clear(seed)
	key := ed25519.NewKeyFromSeed(seed)
	defer clear(key)
	switch action {
	case "public":
		fmt.Println(hex.EncodeToString(key.Public().(ed25519.PublicKey)))
		return nil
	case "sign":
		releaseNotes, err := os.ReadFile(notes)
		if err != nil {
			return err
		}
		manifest, err := appupdate.Sign(key, version, strings.TrimSpace(string(releaseNotes)), setup)
		if err != nil {
			return err
		}
		return os.WriteFile(output, manifest, 0644)
	default:
		return errors.New("unknown action")
	}
}
