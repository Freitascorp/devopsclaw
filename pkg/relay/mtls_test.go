package relay

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateCA(t *testing.T) {
	certPEM, keyPEM, err := GenerateCA("test-org", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if len(certPEM) == 0 {
		t.Error("expected non-empty CA cert PEM")
	}
	if len(keyPEM) == 0 {
		t.Error("expected non-empty CA key PEM")
	}
}

func TestGenerateServerCert(t *testing.T) {
	caCert, caKey, err := GenerateCA("test-org", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	serverCert, serverKey, err := GenerateServerCert(caCert, caKey, []string{"localhost", "127.0.0.1"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}
	if len(serverCert) == 0 {
		t.Error("expected non-empty server cert PEM")
	}
	if len(serverKey) == 0 {
		t.Error("expected non-empty server key PEM")
	}

	// Verify the cert loads
	_, err = tls.X509KeyPair(serverCert, serverKey)
	if err != nil {
		t.Fatalf("server cert/key pair invalid: %v", err)
	}
}

func TestGenerateNodeCert(t *testing.T) {
	caCert, caKey, err := GenerateCA("test-org", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	nodeCert, nodeKey, err := GenerateNodeCert(caCert, caKey, "web-node-01", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateNodeCert: %v", err)
	}
	if len(nodeCert) == 0 {
		t.Error("expected non-empty node cert PEM")
	}
	if len(nodeKey) == 0 {
		t.Error("expected non-empty node key PEM")
	}

	// Verify the cert loads
	_, err = tls.X509KeyPair(nodeCert, nodeKey)
	if err != nil {
		t.Fatalf("node cert/key pair invalid: %v", err)
	}
}

func TestWriteCertFiles(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM, err := GenerateCA("test-org", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	certPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca-key.pem")

	if err := WriteCertFiles(certPath, keyPath, certPEM, keyPEM); err != nil {
		t.Fatalf("WriteCertFiles: %v", err)
	}

	// Check files exist and key has restricted permissions
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("key file permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestServerTLSConfig_mTLS(t *testing.T) {
	dir := t.TempDir()

	// Generate CA
	caCert, caKey, err := GenerateCA("test-org", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caPath := filepath.Join(dir, "ca.pem")
	os.WriteFile(caPath, caCert, 0644)

	// Generate server cert
	serverCertPEM, serverKeyPEM, err := GenerateServerCert(caCert, caKey, []string{"localhost"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}
	serverCertPath := filepath.Join(dir, "server.pem")
	serverKeyPath := filepath.Join(dir, "server-key.pem")
	os.WriteFile(serverCertPath, serverCertPEM, 0644)
	os.WriteFile(serverKeyPath, serverKeyPEM, 0600)

	// Build mTLS config
	cfg := MTLSConfig{
		CACertFile:        caPath,
		ServerCertFile:    serverCertPath,
		ServerKeyFile:     serverKeyPath,
		RequireClientCert: true,
	}

	tlsCfg, err := ServerTLSConfig(cfg)
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}

	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", tlsCfg.ClientAuth)
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %v, want TLS 1.3", tlsCfg.MinVersion)
	}
	if tlsCfg.ClientCAs == nil {
		t.Error("expected non-nil ClientCAs pool")
	}
}

func TestClientTLSConfig_mTLS(t *testing.T) {
	dir := t.TempDir()

	// Generate CA
	caCert, caKey, err := GenerateCA("test-org", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caPath := filepath.Join(dir, "ca.pem")
	os.WriteFile(caPath, caCert, 0644)

	// Generate node cert
	nodeCertPEM, nodeKeyPEM, err := GenerateNodeCert(caCert, caKey, "test-node-01", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateNodeCert: %v", err)
	}
	nodeCertPath := filepath.Join(dir, "node.pem")
	nodeKeyPath := filepath.Join(dir, "node-key.pem")
	os.WriteFile(nodeCertPath, nodeCertPEM, 0644)
	os.WriteFile(nodeKeyPath, nodeKeyPEM, 0600)

	cfg := MTLSConfig{
		CACertFile:     caPath,
		ClientCertFile: nodeCertPath,
		ClientKeyFile:  nodeKeyPath,
	}

	tlsCfg, err := ClientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}

	if tlsCfg.RootCAs == nil {
		t.Error("expected non-nil RootCAs pool")
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 client certificate, got %d", len(tlsCfg.Certificates))
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %v, want TLS 1.3", tlsCfg.MinVersion)
	}
}

func TestExtractNodeIDFromCert(t *testing.T) {
	// nil state
	_, err := ExtractNodeIDFromCert(nil)
	if err == nil {
		t.Error("expected error for nil state")
	}

	// empty state
	_, err = ExtractNodeIDFromCert(&tls.ConnectionState{})
	if err == nil {
		t.Error("expected error for no peer certs")
	}
}

func TestVerifyClientCert_NilState(t *testing.T) {
	_, err := VerifyClientCert(nil)
	if err == nil {
		t.Error("expected error for nil state")
	}
}

func TestVerifyClientCert_NoPeerCerts(t *testing.T) {
	_, err := VerifyClientCert(&tls.ConnectionState{})
	if err == nil {
		t.Error("expected error for no peer certs")
	}
}
