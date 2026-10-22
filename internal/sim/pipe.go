package sim

import (
	"net"
)

// bufferedPipe returns two connected sockets with OS buffering, which
// behaves like a serial port (writes do not wait for the reader), unlike
// net.Pipe.
func bufferedPipe() (net.Conn, net.Conn) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		a, b := net.Pipe()
		return a, b
	}
	defer ln.Close()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := ln.Accept()
		ch <- res{c, err}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		a, b := net.Pipe()
		return a, b
	}
	r := <-ch
	if r.err != nil {
		client.Close()
		a, b := net.Pipe()
		return a, b
	}
	return client, r.c
}
