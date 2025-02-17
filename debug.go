package http321

import (
	"fmt"
	"io"
)

type EchoReader struct {
	io.Reader
}

func (d *EchoReader) Read(p []byte) (n int, err error) {
	defer func() { fmt.Printf("Read bytes: %q\n", string(p[:n])) }()
	return d.Reader.Read(p)
}

type EchoWriter struct {
	io.Writer
}

func (d *EchoWriter) Write(p []byte) (n int, err error) {
	defer func() { fmt.Printf("Write bytes: %q\n", string(p[:n])) }()
	return d.Writer.Write(p)
}
