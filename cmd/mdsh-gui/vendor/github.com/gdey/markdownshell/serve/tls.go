package serve

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TLS is how the server encrypts its traffic and, optionally, checks who
// is connecting.
//
// A server on a loopback address speaks plain HTTP unless given a
// certificate or a client CA. A server any other machine can reach always
// speaks HTTPS: with Cert and Key when given, else with a self-signed
// certificate kept in CertDir, whose fingerprint it prints so the browser's
// warning can be checked against it.
type TLS struct {
	Cert string // a PEM certificate file (with any intermediates), for HTTPS
	Key  string // its PEM private key file
	// ClientCA is a PEM file of the CAs that sign client certificates.
	// When set, a browser must present a certificate one of them signed
	// before it gets as far as the token: mutual TLS.
	ClientCA string
	// CertDir is where a self-signed certificate is kept between runs, so
	// a browser told to trust it once keeps trusting it. "" makes a new
	// one each run.
	CertDir string
}

// selfSignedValid is how long a self-signed certificate lasts. Browsers
// don't apply their 398-day limit to certificates the user trusted by
// hand, but a year keeps it from outliving whatever it was made for.
const selfSignedValid = 365 * 24 * time.Hour

// selfSignedName is the file name of the self-signed certificate and key
// in CertDir.
const selfSignedName = "serve-self-signed"

// tlsConfig is the server's TLS config for o, listening on host, or nil
// for plain HTTP. note says what the user should know about it, such as
// a self-signed certificate's fingerprint.
func tlsConfig(o TLS, host string, local bool) (cfg *tls.Config, note string, err error) {
	if (o.Cert == "") != (o.Key == "") {
		return nil, "", errors.New("--serve-cert and --serve-key go together")
	}
	if o.Cert == "" && o.ClientCA == "" && local {
		return nil, "", nil
	}
	cfg = &tls.Config{MinVersion: tls.VersionTLS12}
	if o.Cert != "" {
		c, err := tls.LoadX509KeyPair(o.Cert, o.Key)
		if err != nil {
			return nil, "", fmt.Errorf("--serve-cert: %w", err)
		}
		cfg.Certificates = []tls.Certificate{c}
	} else {
		c, fresh, err := selfSigned(o.CertDir, host, time.Now())
		if err != nil {
			return nil, "", fmt.Errorf("making a self-signed certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{c}
		sum := sha256.Sum256(c.Certificate[0])
		where := "a new self-signed certificate"
		if !fresh {
			where = "the self-signed certificate from an earlier run"
		}
		note = fmt.Sprintf("using %s; the browser will warn about it until you trust it. Its SHA-256 fingerprint is\n  %s", where, fingerprint(sum[:]))
		if o.CertDir != "" {
			note += "\nIt is kept in " + filepath.Join(o.CertDir, selfSignedName+".pem") + ", which you can import and trust. Give --serve-cert and --serve-key to use your own."
		}
	}
	if o.ClientCA != "" {
		pem, err := os.ReadFile(o.ClientCA)
		if err != nil {
			return nil, "", fmt.Errorf("--serve-client-ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, "", fmt.Errorf("--serve-client-ca: no PEM certificates in %s", o.ClientCA)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, note, nil
}

// fingerprint is sum as colon-separated hex, as browsers show it.
func fingerprint(sum []byte) string {
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// selfSigned is a self-signed certificate for host: the one in dir if it
// is still good for a while and names host, else a new one, saved to dir
// when dir isn't "". fresh says it is new.
func selfSigned(dir, host string, now time.Time) (c tls.Certificate, fresh bool, err error) {
	names, ips := certNames(host)
	certFile := filepath.Join(dir, selfSignedName+".pem")
	keyFile := filepath.Join(dir, selfSignedName+"-key.pem")
	if dir != "" {
		if c, err := tls.LoadX509KeyPair(certFile, keyFile); err == nil && usable(c, names, ips, now) {
			return c, false, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return c, false, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return c, false, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "mdsh --serve", Organization: []string{"mdsh"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(selfSignedValid),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              names,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return c, false, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return c, false, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return c, false, err
		}
		// The key first, and only readable by the user.
		if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
			return c, false, err
		}
		if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
			return c, false, err
		}
	}
	c, err = tls.X509KeyPair(certPEM, keyPEM)
	return c, true, err
}

// usable reports whether c names every one of names and ips and lasts at
// least another week.
func usable(c tls.Certificate, names []string, ips []net.IP, now time.Time) bool {
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil || now.Add(7*24*time.Hour).After(leaf.NotAfter) {
		return false
	}
	for _, n := range names {
		if leaf.VerifyHostname(n) != nil {
			return false
		}
	}
	for _, ip := range ips {
		if leaf.VerifyHostname(ip.String()) != nil {
			return false
		}
	}
	return true
}

// certNames are the names and addresses a self-signed certificate for
// host should be good for: host itself, and for an address that listens
// everywhere, this machine's name and addresses. localhost is always in.
func certNames(host string) (names []string, ips []net.IP) {
	addIP := func(ip net.IP) {
		for _, have := range ips {
			if have.Equal(ip) {
				return
			}
		}
		ips = append(ips, ip)
	}
	addName := func(n string) {
		n = strings.ToLower(strings.TrimSuffix(n, "."))
		for _, have := range names {
			if have == n {
				return
			}
		}
		if n != "" {
			names = append(names, n)
		}
	}
	addName("localhost")
	addIP(net.IPv4(127, 0, 0, 1))
	addIP(net.IPv6loopback)
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil && !ip.IsUnspecified() {
		addIP(ip)
		return names, ips
	} else if ip == nil && host != "" {
		addName(host)
		return names, ips
	}
	// Every address: name what this machine answers to.
	if h, err := os.Hostname(); err == nil {
		addName(h)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && !n.IP.IsLinkLocalUnicast() {
				addIP(n.IP)
			}
		}
	}
	return names, ips
}
