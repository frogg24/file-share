package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"log"
	"os"
)

func main() {
	err := os.MkdirAll("secrets", 0700)
	if err != nil {
		log.Fatal(err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}

	err = saveKey("secrets/auth-private.key", privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = saveKey("secrets/auth-public.key", publicKey)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Keys saved to secrets directory")
}

func saveKey(path string, data []byte) error {
	file, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		return err
	}

	_, writeErr := file.Write(data)
	closeErr := file.Close()

	if writeErr != nil {
		return writeErr
	}

	return closeErr
}
