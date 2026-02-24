// Package relay — mTLS authentication for relay connections.
//
// mTLS (mutual TLS) replaces shared bearer tokens with X.509 certificate-based
// authentication. Both the relay server and node agents present certificates
// signed by a shared CA, providing strong bidirectional identity verification.
//
// Certificate hierarchy:
//
//	[CA cert] ─── signs ──► [Server cert]   (relay server identity)
//	     │
//	     └── signs ──► [Node cert]          (per-node identity, CN=<node-id>)
//
// Usage:
//
//	# Generate CA, server, and node certs:
//	devopsclaw relay cert-gen --ca --out /etc/devopsclaw/certs/
//	devopsclaw relay cert-gen --server --ca-cert ca.pem --ca-key ca-key.pem
//	devopsclaw relay cert-gen --node --node-id web-01 --ca-cert ca.pem --ca-key ca-key.pem
package relay

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

// ------------------------------------------------------------------
// mTLS Configuration
// ------------------------------------------------------------------

// MTLSConfig configures mutual TLS for the relay.
type MTLSConfig struct {
	// Server-side
	CACertFile     string `json:"ca_cert_file"`     // Path to CA certificate (PEM)
	ServerCertFile string `json:"server_cert_file"`  // Path to server certificate (PEM)
	ServerKeyFile  string `json:"server_key_file"`   // Path to server private key (PEM)

	// Client-side (node agent)
	ClientCertFile string `json:"client_cert_file"` // Path to node certificate (PEM)
	ClientKeyFile  string `json:"client_key_file"`  // Path to node private key (PEM)

	// Policy
	RequireClientCert bool `json:"require_client_cert"` // Reject connections without valid client cert
	AllowTokenFallback bool `json:"allow_token_fallback"` // Allow bearer token if no client cert (migration mode)
}

// ServerTLSConfig builds a *tls.Config for the relay server with mTLS.
// The server presents its own cert and requires clients to present a CA-signed cert.
func ServerTLSConfig(cfg MTLSConfig) (*tls.Config, error) {
	// Load CA certificate pool
	caCert, err := os.ReadFile(cfg.CACertFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert %s: %w", cfg.CACertFile, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate from %s", cfg.CACertFile)
	}

	// Load server certificate and key
	serverCert, err := tls.LoadX509KeyPair(cfg.ServerCertFile, cfg.ServerKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert/key: %w", err)
	}

	clientAuth := tls.RequireAndVerifyClientCert
	if !cfg.RequireClientCert {
		clientAuth = tls.VerifyClientCertIfGiven
	}

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   clientAuth,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ClientTLSConfig builds a *tls.Config for a node agent connecting to the relay.
// The agent presents its own cert and verifies the server's cert against the CA.
func ClientTLSConfig(cfg MTLSConfig) (*tls.Config, error) {
	// Load CA certificate pool (to verify server)
	caCert, err := os.ReadFile(cfg.CACertFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert %s: %w", cfg.CACertFile, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate from %s", cfg.CACertFile)
	}

	// Load client certificate and key
	clientCert, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ExtractNodeIDFromCert extracts the node ID from a verified client certificate.
// The node ID is stored in the certificate's Common Name (CN) field.
func ExtractNodeIDFromCert(state *tls.ConnectionState) (string, error) {
	if state == nil || len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("no client certificate presented")
	}
	cn := state.PeerCertificates[0].Subject.CommonName
	if cn == "" {
		return "", fmt.Errorf("client certificate has empty Common Name")
	}
	return cn, nil
}

// VerifyClientCert checks that the client certificate is valid and
// extracts identity information. Used in the WebSocket handler.
func VerifyClientCert(state *tls.ConnectionState) (*ClientIdentity, error) {
	if state == nil {
		return nil, fmt.Errorf("no TLS connection state")
	}

	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no client certificate presented")
	}

	cert := state.PeerCertificates[0]
	if cert.Subject.CommonName == "" {
		return nil, fmt.Errorf("certificate CN is empty")
	}

	// Check expiry
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, fmt.Errorf("certificate expired or not yet valid (valid %s to %s)",
			cert.NotBefore.Format(time.RFC3339), cert.NotAfter.Format(time.RFC3339))
	}

	return &ClientIdentity{
		NodeID:      cert.Subject.CommonName,
		Fingerprint: fmt.Sprintf("%x", cert.Signature[:16]),
		Organization: firstOrEmpty(cert.Subject.Organization),
		ValidUntil:  cert.NotAfter,
	}, nil
}

// ClientIdentity holds verified identity from a client certificate.
type ClientIdentity struct {
	NodeID       string    `json:"node_id"`
	Fingerprint  string    `json:"fingerprint"`
	Organization string    `json:"organization"`
	ValidUntil   time.Time `json:"valid_until"`
}

func firstOrEmpty(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// ------------------------------------------------------------------
// Certificate Generation Helpers
// ------------------------------------------------------------------

// GenerateCA creates a self-signed CA certificate and private key.
func GenerateCA(org string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{org},
			CommonName:   org + " CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create CA cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal CA key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// GenerateServerCert creates a server certificate signed by the given CA.
func GenerateServerCert(caCertPEM, caKeyPEM []byte, hosts []string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	caCert, caKey, err := parseCA(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate server key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: hosts[0],
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(validFor),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create server cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal server key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// GenerateNodeCert creates a client certificate for a fleet node, signed by the CA.
// The nodeID is embedded as the certificate's Common Name.
func GenerateNodeCert(caCertPEM, caKeyPEM []byte, nodeID string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	caCert, caKey, err := parseCA(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate node key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   nodeID,
			Organization: []string{"devopsclaw-fleet"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(validFor),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create node cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal node key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// WriteCertFiles writes cert and key PEM files to disk.
func WriteCertFiles(certPath, keyPath string, certPEM, keyPEM []byte) error {
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("write cert %s: %w", certPath, err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("write key %s: %w", keyPath, err)
	}
	return nil
}

func parseCA(caCertPEM, caKeyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(caCertPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA cert: %w", err)
	}

	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA key PEM")
	}
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA key: %w", err)
	}

	return caCert, caKey, nil
}
