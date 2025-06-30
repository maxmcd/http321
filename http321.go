package http321

import (
	"context"
	"crypto/tls"
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
	return &ReadWriteConn{Reader: stream, Writer: stream, Closer: stream}, nil
	// return &ReadWriteConn{Reader: &EchoReader{stream}, Writer: &EchoWriter{stream}, Closer: stream}, nil
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
		l.conns = make(chan net.Conn, 1)
		handler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			pReader, pWriter := io.Pipe()
			l.conns <- &ReadWriteConn{Reader: r.Body, Writer: pWriter, Closer: r.Body}
			_, _ = io.Copy(flushWriter{w}, pReader)
		}), &http2.Server{})
		go func() {
			for {
				conn, err := l.listener.Accept()
				if err != nil {
					break
				}
				go (&http2.Server{}).ServeConn(conn, &http2.ServeConnOpts{
					Handler: handler,
				})
			}
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

func HTTP2OverQuicDial(conn quic.Connection) (net.Conn, error) {
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true, // Enable h2c support
			DialTLSContext: func(ctx context.Context,
				network, addr string, cfg *tls.Config) (net.Conn, error) {
				return QuicConnDial(conn)
			},
		},
	}
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	req, err := http.NewRequest(http.MethodPost, "http://whatever", io.NopCloser(inReader))
	if err != nil {
		return nil, err
	}
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("HTTP2OverQuicDial error", err)
		}
		_, _ = io.Copy(outWriter, resp.Body)
	}()
	return &ReadWriteConn{Reader: outReader, Writer: inWriter, Closer: outReader}, nil
}
