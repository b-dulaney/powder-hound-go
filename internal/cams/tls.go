package cams

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Some cam servers (Brownrice's streamer hosts) send only their leaf certificate, without the
// intermediate. Browsers fill the gap by fetching the intermediate from the leaf's Authority
// Information Access URL; Go doesn't, so the default client fails with "unknown authority".
// aiaVerifier does the same as browsers: normal verification first, and on an unknown-authority
// failure it downloads the issuers named in the leaf and verifies again. The downloaded
// certificates are only used as intermediates, so the chain must still end at a trusted root.

const maxAIACertBytes = 64 << 10

type aiaVerifier struct {
	// roots is nil in production (system roots); tests set their own.
	roots  *x509.CertPool
	client *http.Client

	mu    sync.Mutex
	cache map[string]*x509.Certificate
}

func newAIAVerifier(roots *x509.CertPool) *aiaVerifier {
	return &aiaVerifier{
		roots:  roots,
		client: &http.Client{Timeout: 10 * time.Second},
		cache:  map[string]*x509.Certificate{},
	}
}

// tlsConfig returns a client config that verifies servers with AIA fetching. Verification is
// done in VerifyConnection (InsecureSkipVerify only disables the built-in check it replaces).
func (v *aiaVerifier) tlsConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection:   v.verify,
		MinVersion:         tls.VersionTLS12,
	}
}

func (v *aiaVerifier) verify(cs tls.ConnectionState) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("tls: server sent no certificates")
	}
	// ServerName is empty when the URL's host is an IP address, and an empty DNSName would
	// skip the hostname check. Cam URLs are always hostnames, so refuse rather than check less
	// than Go's default verification does.
	if cs.ServerName == "" {
		return errors.New("tls: no server name to verify (cam URLs must use a hostname)")
	}
	leaf := cs.PeerCertificates[0]
	opts := x509.VerifyOptions{
		DNSName:       cs.ServerName,
		Roots:         v.roots,
		Intermediates: x509.NewCertPool(),
	}
	for _, c := range cs.PeerCertificates[1:] {
		opts.Intermediates.AddCert(c)
	}
	_, err := leaf.Verify(opts)
	var unknown x509.UnknownAuthorityError
	if err == nil || !errors.As(err, &unknown) || len(leaf.IssuingCertificateURL) == 0 {
		return err
	}
	for _, url := range leaf.IssuingCertificateURL {
		cert, fetchErr := v.issuer(url)
		if fetchErr != nil {
			return fmt.Errorf("%w (and fetching the issuer from %s failed: %v)", err, url, fetchErr)
		}
		opts.Intermediates.AddCert(cert)
	}
	_, err = leaf.Verify(opts)
	return err
}

// issuer downloads and caches a certificate from an AIA URL (DER, or PEM from some CAs).
func (v *aiaVerifier) issuer(url string) (*x509.Certificate, error) {
	v.mu.Lock()
	cert, ok := v.cache[url]
	v.mu.Unlock()
	if ok {
		return cert, nil
	}

	resp, err := v.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAIACertBytes))
	if err != nil {
		return nil, err
	}
	if block, _ := pem.Decode(data); block != nil {
		data = block.Bytes
	}
	cert, err = x509.ParseCertificate(data)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	v.mu.Lock()
	v.cache[url] = cert
	v.mu.Unlock()
	return cert, nil
}
