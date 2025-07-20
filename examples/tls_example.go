package main

import (
	"github.com/herro-labs/cargo"
)

func main() {
	app := cargo.New()

	// Add middleware
	app.Use(
		cargo.Logger(),
		cargo.Recovery(),
		cargo.Auth(),
	)

	// Configure TLS with certificate and key files
	app.WithTLS("server.crt", "server.key")

	// For mutual TLS (client certificate verification):
	// app.WithMutualTLS("server.crt", "server.key", "ca.crt")

	// Register your services here...
	// app.RegisterService(&proto.YourService_ServiceDesc)

	// Start server with TLS on port 443
	app.Run(":443")
}
