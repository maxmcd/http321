package http321

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// type quicListener struct {
// 	conn quic.Connection
// }

// func (l *quicListener) Accept() (net.Conn, error) {
// 	stream, err := l.conn.AcceptStream(context.Background())
// 	if err != nil {
// 		return nil, fmt.Errorf("quicListener accept: %w", err)
// 	}

// 	if _, err := parseAddrFromStream(stream); err != nil {
// 		return nil, fmt.Errorf("quicListener parseAddrFromStream: %w", err)
// 	}
// 	_ = binary.Write(stream, binary.BigEndian, uint16(0))

// 	return &streamConn{stream: stream, ReadWriteCloser: stream}, nil
// }

// func (l *quicListener) Close() error {
// 	fmt.Println("closing quic listener")
// 	return l.conn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
// }

//	func (l *quicListener) Addr() net.Addr {
//		return l.conn.LocalAddr()
//	}
func TestHTTP3(t *testing.T) {
	listener := NewHTTP3Listener(t)
	conn := DialListener(t, listener)

	// Accept a client connection.
	serverConn, err := listener.Accept(context.Background())
	if err != nil {
		log.Panicln(err)
	}
	defer func() { _ = serverConn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "") }()

	// Open a stream to the server.
	stream, err := conn.OpenStream()
	if err != nil {
		log.Panicln(err)
	}

	_, _ = stream.Write([]byte("hello"))
	serverStream, err := serverConn.AcceptStream(context.Background())
	if err != nil {
		log.Panicln(err)
	}
	defer func() { _ = serverStream.Close() }()

	buf := make([]byte, 5)
	if _, err := io.ReadFull(serverStream, buf); err != nil {
		log.Panicln(err)
	}

	fmt.Println(string(buf)) // => "hello"
}

func TestHTTP32(t *testing.T) {
	listener := NewHTTP3Listener(t)
	conn := DialListener(t, listener)

	// Accept a client connection.
	serverConn, err := listener.Accept(context.Background())
	if err != nil {
		log.Panicln(err)
	}
	defer func() { _ = serverConn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "") }()

	netListener := QuicNetListener{Connection: serverConn}

	// Configure HTTP/2
	h2conf := &http2.Server{
		IdleTimeout: 1 * time.Hour,
	}
	count := 0
	handler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count += 1
		fmt.Fprintf(w, "Hello from http2! %d", count)
		time.Sleep(time.Millisecond * 100)
	}), h2conf)
	go func() {
		for {
			conn, err := netListener.Accept()
			if err != nil {
				return
			}
			go (&http2.Server{}).ServeConn(conn, &http2.ServeConnOpts{
				Handler: handler,
			})
		}
	}()

	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true, // Enable h2c support
			DialTLSContext: func(ctx context.Context,
				network, addr string, cfg *tls.Config) (net.Conn, error) {
				fmt.Println("dialing new conn")
				return QuicConnDial(conn)
			},
		},
	}

	var wg sync.WaitGroup
	var results []string
	var lock sync.Mutex
	start := time.Now()
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			resp, err := client.Get("http://anything")
			if err != nil {
				panic(err)
			}
			fmt.Println(resp)
			b, _ := io.ReadAll(resp.Body)
			lock.Lock()
			results = append(results, string(b))
			lock.Unlock()
			wg.Done()
		}()
	}
	wg.Wait()
	fmt.Println(results, time.Since(start))
}

func TestHTTP322_2(t *testing.T) {
	listener := NewHTTP3Listener(t)
	conn := DialListener(t, listener)

	// Accept a client connection.
	serverConn, err := listener.Accept(context.Background())
	if err != nil {
		log.Panicln(err)
	}
	defer func() { _ = serverConn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "") }()

	netListener := QuicNetListener{Connection: serverConn}

	// Configure HTTP/2
	h2conf := &http2.Server{
		IdleTimeout: 1 * time.Hour,
	}
	handler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("got request")
		w.WriteHeader(200)
		_, _ = io.Copy(flushWriter{w}, r.Body)
	}), h2conf)
	http2Server := &http.Server{
		Handler: handler,
	}
	go func() {
		_ = http2Server.Serve(&netListener)
	}()

	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true, // Enable h2c support
			DialTLSContext: func(ctx context.Context,
				network, addr string, cfg *tls.Config) (net.Conn, error) {
				fmt.Println("dialing new conn")
				return QuicConnDial(conn)
			},
		},
	}

	pReader, pWriter := io.Pipe()
	req, err := http.NewRequest(http.MethodPost, "http://whatever", io.NopCloser(pReader))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Fprintf(pWriter, "It is now %v\n", time.Now())
		}
		pWriter.Close()
	}()
	fmt.Println("hi")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(os.Stdout, resp.Body)

}

func TestHTTP321(t *testing.T) {
	for i := 0; i < 10; i++ {
		listener := NewHTTP3Listener(t)
		conn := DialListener(t, listener)

		// Accept a client connection.
		serverConn, err := listener.Accept(context.Background())
		if err != nil {
			log.Panicln(err)
		}
		defer func() { _ = serverConn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "") }()

		netListener := QuicNetListener{Connection: serverConn}
		http2Listener := HTTP2OverQuicListener{listener: &netListener}

		go func() {
			_ = http.Serve(&http2Listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Println("got request http1")
				fmt.Fprintf(w, "I'm http1!")
			}))
		}()
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context,
					network, addr string) (net.Conn, error) {
					con, err := HTTP2OverQuicDial(conn)
					return con, err
				},
			},
		}
		resp, err := client.Get("http://whatever")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(resp)
		_, _ = io.Copy(os.Stdout, resp.Body)

		_ = listener.Close()
		_ = conn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
	}
}

func TestHTTP_WS(t *testing.T) {
	t.Skip("Websockets need SetReadDeadline and we don't have that")
	listener := NewHTTP3Listener(t)
	conn := DialListener(t, listener)

	// Accept a client connection.
	serverConn, err := listener.Accept(context.Background())
	if err != nil {
		log.Panicln(err)
	}
	defer func() { _ = serverConn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "") }()

	netListener := QuicNetListener{Connection: serverConn}
	http2Listener := HTTP2OverQuicListener{listener: &netListener}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	go func() {
		_ = http.Serve(&http2Listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// locks here because we don't implement SetReadDeadline and the pending read is never release
			c, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Logf("upgrade error: %v", err)
				return
			}
			defer c.Close()

			// Echo back any messages received
			for {
				mt, message, err := c.ReadMessage()
				if err != nil {
					break
				}
				if err := c.WriteMessage(mt, message); err != nil {
					break
				}
			}
		}))
	}()

	// Create WebSocket client
	dialer := websocket.Dialer{
		NetDialContext: func(ctx context.Context,
			network, addr string) (net.Conn, error) {
			con, err := HTTP2OverQuicDial(conn)
			return con, err
		},
	}

	ws, _, err := dialer.Dial("ws://whatever", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Send a test message
	if err := ws.WriteMessage(websocket.TextMessage, []byte("hello websocket")); err != nil {
		t.Fatal(err)
	}

	// Read the response
	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Received: %s\n", msg)

	listener.Close()
	_ = conn.CloseWithError(quic.ApplicationErrorCode(quic.NoError), "")
}
