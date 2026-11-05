package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/settings"
)

// A loopback server speaks plain HTTP unless told otherwise; anything
// else gets HTTPS, self-signed when no certificate is given.
func TestTLSConfig(t *testing.T) {
	cfg, _, err := tlsConfig(TLS{}, "127.0.0.1", true)
	if err != nil || cfg != nil {
		t.Errorf("loopback: %v, %v; want plain HTTP", cfg, err)
	}
	if _, _, err := tlsConfig(TLS{Cert: "c.pem"}, "127.0.0.1", true); err == nil {
		t.Error("a certificate without its key: no error")
	}
	dir := t.TempDir()
	cfg, note, err := tlsConfig(TLS{CertDir: dir}, "0.0.0.0", false)
	if err != nil || cfg == nil || len(cfg.Certificates) != 1 {
		t.Fatalf("every address: %v, %v", cfg, err)
	}
	if !strings.Contains(note, "new self-signed certificate") || !strings.Contains(note, "SHA-256 fingerprint") {
		t.Errorf("note = %q", note)
	}
	if fi, err := os.Stat(filepath.Join(dir, selfSignedName+"-key.pem")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("key file: %v, %v", fi, err)
	}
	leaf, _ := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Errorf("not good for localhost: %v", err)
	}
	// A second run keeps the same certificate, so a browser told to
	// trust it keeps trusting it.
	cfg2, note2, err := tlsConfig(TLS{CertDir: dir}, "0.0.0.0", false)
	if err != nil || !strings.Contains(note2, "from an earlier run") || string(cfg2.Certificates[0].Certificate[0]) != string(cfg.Certificates[0].Certificate[0]) {
		t.Errorf("second run: %v, note %q", err, note2)
	}
	// One for another host is made again.
	cfg3, note3, err := tlsConfig(TLS{CertDir: dir}, "192.0.2.7", false)
	if err != nil || !strings.Contains(note3, "new self-signed") {
		t.Fatalf("another host: %v, %q", err, note3)
	}
	leaf, _ = x509.ParseCertificate(cfg3.Certificates[0].Certificate[0])
	if err := leaf.VerifyHostname("192.0.2.7"); err != nil {
		t.Errorf("not good for its address: %v", err)
	}
}

// The certificates a mutual TLS test needs: a CA, a client certificate
// it signed, and one some other CA signed.
type testCerts struct {
	caFile          string
	client, unknown tls.Certificate
}

func makeCerts(t *testing.T) testCerts {
	t.Helper()
	newCA := func() (*x509.Certificate, *ecdsa.PrivateKey) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "test CA"},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			IsCA:                  true,
			KeyUsage:              x509.KeyUsageCertSign,
			BasicConstraintsValid: true,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		ca, _ := x509.ParseCertificate(der)
		return ca, key
	}
	clientOf := func(ca *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject:      pkix.Name{CommonName: "laptop"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	}
	ca, caKey := newCA()
	other, otherKey := newCA()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o644); err != nil {
		t.Fatal(err)
	}
	return testCerts{caFile: caFile, client: clientOf(ca, caKey), unknown: clientOf(other, otherKey)}
}

// With a client CA, only a browser with a certificate it signed gets as
// far as the token, and the token's cookie is Secure.
func TestMutualTLS(t *testing.T) {
	certs := makeCerts(t)
	store, err := settings.Open(filepath.Join(t.TempDir(), "gui.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Addr: "127.0.0.1:0", Settings: store, Cwd: t.TempDir(), Token: "secret", TLS: TLS{ClientCA: certs.caFile}})
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if u := s.URL(l); !strings.HasPrefix(u, "https://") {
		t.Errorf("URL = %q, want https", u)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	client := func(cert *tls.Certificate) *http.Client {
		tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // the server's is self-signed
		if cert != nil {
			tr.TLSClientConfig.Certificates = []tls.Certificate{*cert}
		}
		jar, _ := cookiejar.New(nil)
		return &http.Client{Transport: tr, Jar: jar, Timeout: 5 * time.Second}
	}
	base := "https://" + l.Addr().String()
	for name, c := range map[string]*http.Client{"no certificate": client(nil), "another CA's": client(&certs.unknown)} {
		if resp, err := c.Get(base + "/?token=secret"); err == nil {
			resp.Body.Close()
			t.Errorf("%s: got %s, want the handshake refused", name, resp.Status)
		}
	}
	c := client(&certs.client)
	resp, err := c.Get(base + "/?token=secret")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("with the certificate and token: %s", resp.Status)
	}
	var secure bool
	for r := resp.Request.Response; r != nil; r = r.Request.Response {
		for _, ck := range r.Cookies() {
			secure = secure || ck.Secure
		}
	}
	if !secure {
		t.Error("the token's cookie isn't Secure")
	}
	// The certificate alone isn't enough: the token is still needed.
	resp, err = client(&certs.client).Get(base + "/style.css")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("certificate without the token: %s", resp.Status)
	}
}
