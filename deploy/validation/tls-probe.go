// Command tls-probe performs one hostname-validating TLS handshake for the
// disposable Phase 3 Caddy validation. Production certificate observation uses
// the system trust store; this helper accepts only the isolated Caddy test CA.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	address := flag.String("address", "", "TLS address in host:port form")
	hostname := flag.String("hostname", "", "expected certificate hostname")
	caPath := flag.String("ca", "", "PEM root CA path")
	flag.Parse()
	if *address == "" || *hostname == "" || *caPath == "" {
		fmt.Fprintln(os.Stderr, "address, hostname, and ca are required")
		os.Exit(2)
	}
	pem, err := os.ReadFile(*caPath)
	if err != nil {
		panic(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		panic("test CA does not contain a certificate")
	}
	dialer := &tls.Dialer{Config: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: *hostname,
	}}
	connection, err := dialer.Dial("tcp", *address)
	if err != nil {
		panic(err)
	}
	defer connection.Close()
	state := connection.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		panic("server returned no certificate")
	}
	certificate := state.PeerCertificates[0]
	if err := certificate.VerifyHostname(*hostname); err != nil {
		panic(err)
	}
	if !time.Now().Before(certificate.NotAfter) {
		panic("certificate is expired")
	}
	fmt.Printf("TLS_PROBE_OK hostname=%s expires=%s\n", *hostname, certificate.NotAfter.UTC().Format(time.RFC3339))
}
