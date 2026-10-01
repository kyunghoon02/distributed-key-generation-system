package main

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
	"path/filepath"
	"time"
)

type realTLSCredentials struct {
	dir        string
	caCert     string
	serverCert [4]string
	serverKey  [4]string
	roots      *x509.CertPool
	client     tls.Certificate
}

func newLocalRealTLSCredentials() (credentials realTLSCredentials, err error) {
	credentials.dir, err = os.MkdirTemp("", "dkgctl-real-tls-")
	if err != nil {
		return credentials, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(credentials.dir)
		}
	}()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return credentials, err
	}
	now := time.Now()
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1),
		Subject:   pkix.Name{CommonName: "dkgctl local ceremony CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return credentials, err
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return credentials, err
	}
	credentials.caCert = filepath.Join(credentials.dir, "ca.pem")
	if err = os.WriteFile(credentials.caCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		return credentials, err
	}
	credentials.roots = x509.NewCertPool()
	credentials.roots.AddCert(ca)
	clientCert := filepath.Join(credentials.dir, "controller.pem")
	clientKey := filepath.Join(credentials.dir, "controller-key.pem")
	if err = writeRealTLSLeaf(clientCert, clientKey, "dkg-controller", x509.ExtKeyUsageClientAuth, nil, ca, caKey); err != nil {
		return credentials, err
	}
	credentials.client, err = tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return credentials, err
	}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("p%d", i+1)
		credentials.serverCert[i] = filepath.Join(credentials.dir, id+".pem")
		credentials.serverKey[i] = filepath.Join(credentials.dir, id+"-key.pem")
		if err = writeRealTLSLeaf(credentials.serverCert[i], credentials.serverKey[i], id, x509.ExtKeyUsageServerAuth, net.ParseIP("127.0.0.1"), ca, caKey); err != nil {
			return credentials, err
		}
	}
	return credentials, nil
}

func writeRealTLSLeaf(certPath, keyPath, name string, usage x509.ExtKeyUsage, ip net.IP, ca *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		DNSNames: []string{name}}
	if ip != nil {
		template.IPAddresses = []net.IP{ip}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600)
}

func (c realTLSCredentials) clientConfig(serverName string) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: c.roots,
		Certificates: []tls.Certificate{c.client}, ServerName: serverName}
}
