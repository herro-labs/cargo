package cargo

import (
	"crypto/tls"
	"net"

	"google.golang.org/grpc/credentials"
)

// TLSConfig holds TLS configuration for the server
type TLSConfig struct {
	CertFile string // Path to certificate file
	KeyFile  string // Path to private key file
	ClientCA string // Path to client CA file (for mutual TLS)
}

// Note: WithTLS and WithMutualTLS methods are now defined in cargo.go
// These methods are kept here for reference but should be removed

// createTLSCredentials creates gRPC TLS credentials from the TLS config
func (a *App) createTLSCredentials() (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(a.config.TLS.CertFile, a.config.TLS.KeyFile)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}

	// If client CA is specified, enable mutual TLS
	if a.config.TLS.ClientCA != "" {
		config.ClientAuth = tls.RequireAndVerifyClientCert
		// Note: In a full implementation, you'd load the client CA here
	}

	return credentials.NewTLS(config), nil
}

// createSecureListener creates a TLS listener if TLS is configured
func (a *App) createSecureListener() (net.Listener, error) {
	if a.config.TLS.CertFile == "" {
		// No TLS configured, use plain TCP
		return net.Listen("tcp", a.config.Server.Port)
	}

	cert, err := tls.LoadX509KeyPair(a.config.TLS.CertFile, a.config.TLS.KeyFile)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	return tls.Listen("tcp", a.config.Server.Port, config)
}
