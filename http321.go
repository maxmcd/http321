package http321

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// type DebugReadWriteCloser struct {
// 	wrapped io.ReadWriteCloser
// }

// func (d *DebugReadWriteCloser) Read(p []byte) (n int, err error) {
// 	defer func() { fmt.Printf("Read bytes: %q\n", string(p[:n])) }()
// 	return d.wrapped.Read(p)
// }

// func (d *DebugReadWriteCloser) Write(p []byte) (n int, err error) {
// 	defer func() { fmt.Printf("Write bytes: %q\n", string(p[:n])) }()
// 	return d.wrapped.Write(p)
// }

// func (d *DebugReadWriteCloser) Close() error {
// 	return d.wrapped.Close()
// }

// QuicNetListener implements a net.Listener on top of a quic.Connection
type QuicNetListener struct {
	quic.Connection
}

var _ net.Listener = &QuicNetListener{}

func (l *QuicNetListener) Accept() (net.Conn, error) {
	stream, err := l.Connection.AcceptStream(context.Background())
	if err != nil {
		return nil, fmt.Errorf("quicListener accept: %w", err)
	}
	fmt.Println("new stream on listener")
	return &ReadWriteConn{Reader: stream, Writer: stream, Closer: stream}, nil
}

func (l *QuicNetListener) Close() error {
	return l.Connection.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
}

func (l *QuicNetListener) Addr() net.Addr {
	return l.Connection.LocalAddr()
}

// ReadWriteConn implement net.Conn for a quic.Stream
type ReadWriteConn struct {
	io.Reader
	io.Writer
	io.Closer
}

func (c *ReadWriteConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 0} }
func (c *ReadWriteConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 0} }
func (c *ReadWriteConn) SetDeadline(t time.Time) error      { return nil }
func (c *ReadWriteConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *ReadWriteConn) SetWriteDeadline(t time.Time) error { return nil }

func QuicConnDial(conn quic.Connection) (net.Conn, error) {
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return nil, fmt.Errorf("quicConnDial openStreamSync: %w", err)
	}
	return &ReadWriteConn{Reader: stream, Writer: stream, Closer: stream}, nil
}

type flushWriter struct {
	w io.Writer
}

func (fw flushWriter) Write(p []byte) (n int, err error) {
	fmt.Println("writing", string(p))
	n, err = fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return
}

type HTTP2OverQuicListener struct {
	once     sync.Once
	listener *QuicNetListener
	conns    chan net.Conn
	server   *http.Server
}

var _ net.Listener = &HTTP2OverQuicListener{}

func (l *HTTP2OverQuicListener) Accept() (net.Conn, error) {
	l.once.Do(func() {
		h2conf := &http2.Server{
			IdleTimeout: 1 * time.Hour,
		}
		handler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Println("got request")
			w.WriteHeader(200)
			pReader, pWriter := io.Pipe()
			l.conns <- &ReadWriteConn{Reader: r.Body, Writer: pWriter, Closer: r.Body}
			_, _ = io.Copy(flushWriter{w}, pReader)
		}), h2conf)
		http2Server := &http.Server{
			Handler: handler,
		}
		go func() {
			_ = http2Server.Serve(l.listener)
		}()
	})
	return <-l.conns, nil
}

func (l *HTTP2OverQuicListener) Close() error {
	if l.server != nil {
		return l.server.Close()
	}
	return nil
}

func (l *HTTP2OverQuicListener) Addr() net.Addr {
	return l.listener.Addr()
}
