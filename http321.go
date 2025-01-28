package http321

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// QuicNetListener implements a net.Listener on top of a quic.Connection
type QuicNetListener struct {
	quic.Connection
}

func (l *QuicNetListener) Accept() (net.Conn, error) {
	stream, err := l.Connection.AcceptStream(context.Background())
	if err != nil {
		return nil, fmt.Errorf("quicListener accept: %w", err)
	}
	fmt.Println("new stream on listener")
	return &StreamConn{stream: stream, ReadWriteCloser: stream}, nil
}

func (l *QuicNetListener) Close() error {
	return l.Connection.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
}

func (l *QuicNetListener) Addr() net.Addr {
	return l.Connection.LocalAddr()
}

// StreamConn implement net.Conn for a quic.Stream
type StreamConn struct {
	stream quic.Stream
	io.ReadWriteCloser
}

func (c *StreamConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 0} }
func (c *StreamConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 0} }
func (c *StreamConn) SetDeadline(t time.Time) error      { return nil }
func (c *StreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *StreamConn) SetWriteDeadline(t time.Time) error { return nil }

func QuicConnDial(conn quic.Connection) (net.Conn, error) {
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return nil, fmt.Errorf("quicConnDial openStreamSync: %w", err)
	}
	return &StreamConn{stream: stream, ReadWriteCloser: stream}, nil
}
