package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newTestKey(t *testing.T) (privatePEM string, signer ssh.Signer) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	signer, err = ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), signer
}

func authorizedKey(signer ssh.Signer) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
}

func TestSignCertWithPrivateKey_doesNotLeakKeyMaterial(t *testing.T) {
	const marker = "SECRET-KEY-MARKER"
	pemKey, signer := newTestKey(t)

	cases := map[string]struct{ key, cert string }{
		"unparseable private key": {key: marker, cert: authorizedKey(signer)},
		"unparseable certificate": {key: pemKey, cert: marker},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := signCertWithPrivateKey(c.key, c.cert)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatalf("error leaks its input: %v", err)
			}
		})
	}
}

func TestSignCertWithPrivateKey_rejectsPlainPublicKey(t *testing.T) {
	pemKey, signer := newTestKey(t)

	// A parseable public key that is not a certificate must be an error, not a panic.
	if _, err := signCertWithPrivateKey(pemKey, authorizedKey(signer)); err == nil {
		t.Fatal("expected an error for a non-certificate public key")
	}
}
