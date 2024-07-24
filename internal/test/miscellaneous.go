package test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"testing"

	. "github.com/onsi/gomega"
)

// ReadFile reads the file present at the given filePath and returns a byte Buffer containing its contents.
func ReadFile(filePath string) (*bytes.Buffer, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(f)
	if err != nil {
		return nil, err
	}
	return buff, nil
}

// FileExistsOrFail checks if the given filepath is valid and returns an error if file is not found or does not exist.
func FileExistsOrFail(filepath string) {
	var err error
	if _, err = os.Stat(filepath); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", filepath)
	}
	if err != nil {
		log.Fatalf("Error occured in finding file %s : %v", filepath, err)
	}
}

// ValidateIfFileExists validates the existence of a file
func ValidateIfFileExists(file string, t *testing.T) {
	g := NewWithT(t)
	var err error
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", file)
	}
	g.Expect(err).ToNot(HaveOccurred(), "File at path %v should exist")
}

// GenerateRandomAlphanumericString generates a random alphanumeric string of the given length.
func GenerateRandomAlphanumericString(g *WithT, length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	g.Expect(err).ToNot(HaveOccurred())
	return hex.EncodeToString(b)
}