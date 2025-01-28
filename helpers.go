package http321

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"testing"

	"github.com/quic-go/quic-go"
)

var tlsConfig = loadTLSConfig()

func loadTLSConfig() *tls.Config {
	tlsCert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		log.Panicln(fmt.Errorf("loading certificates: %w", err))
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tlsCert},
	}
	return tlsConfig
}

func NewHTTP3Listener(t *testing.T) *quic.Listener {
	// Start the server.
	listener, err := quic.ListenAddr("127.0.0.1:0", tlsConfig, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

func DialListener(t *testing.T, listener *quic.Listener) quic.Connection {
	// Connect a client.
	conn, err := quic.DialAddr(context.Background(), listener.Addr().String(), tlsConfig, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "") })
	return conn
}
