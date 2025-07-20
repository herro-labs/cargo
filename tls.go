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

// WithTLS configures the app to use TLS with the provided certificate and key files
func (a *App) WithTLS(certFile, keyFile string) *App {
	a.tlsConfig = &TLSConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
	}
	return a
}

// WithMutualTLS configures the app to use mutual TLS (client certificate verification)
func (a *App) WithMutualTLS(certFile, keyFile, clientCA string) *App {
	a.tlsConfig = &TLSConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
		ClientCA: clientCA,
	}
	return a
}

// createTLSCredentials creates gRPC TLS credentials from the TLS config
func (a *App) createTLSCredentials() (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(a.tlsConfig.CertFile, a.tlsConfig.KeyFile)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}

	// If client CA is specified, enable mutual TLS
	if a.tlsConfig.ClientCA != "" {
		config.ClientAuth = tls.RequireAndVerifyClientCert
		// Note: In a full implementation, you'd load the client CA here
	}

	return credentials.NewTLS(config), nil
}

// createSecureListener creates a TLS listener if TLS is configured
func (a *App) createSecureListener() (net.Listener, error) {
	if a.tlsConfig == nil {
		// No TLS configured, use plain TCP
		return net.Listen("tcp", a.port)
	}

	cert, err := tls.LoadX509KeyPair(a.tlsConfig.CertFile, a.tlsConfig.KeyFile)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	return tls.Listen("tcp", a.port, config)
}
