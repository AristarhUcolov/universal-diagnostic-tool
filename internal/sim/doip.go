package sim

import (
	"encoding/binary"
	"net"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/doip"
)

// DoIPServer is a simulated DoIP gateway with one UDS control unit
// behind it (logical address 0x1000 by default).
type DoIPServer struct {
	ln      net.Listener
	App     App
	Logical uint16
}

// StartDoIP listens on addr ("127.0.0.1:0" for a random port).
func StartDoIP(addr string, app App) (*DoIPServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &DoIPServer{ln: ln, App: app, Logical: 0x1000}
	go s.serve()
	return s, nil
}

// Addr returns the listening address.
func (s *DoIPServer) Addr() string { return s.ln.Addr().String() }

// Close stops the server.
func (s *DoIPServer) Close() error { return s.ln.Close() }

func (s *DoIPServer) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *DoIPServer) handle(c net.Conn) {
	defer c.Close()
	var tester uint16
	for {
		p, err := doip.ReadPacket(c)
		if err != nil {
			return
		}
		switch p.Type {
		case doip.TypeRoutingRequest:
			if len(p.Payload) < 2 {
				return
			}
			tester = binary.BigEndian.Uint16(p.Payload)
			resp := make([]byte, 9)
			binary.BigEndian.PutUint16(resp, tester)
			binary.BigEndian.PutUint16(resp[2:], s.Logical)
			resp[4] = 0x10
			c.Write(doip.Packet{Type: doip.TypeRoutingResponse, Payload: resp}.Encode())
		case doip.TypeDiagnosticMessage:
			if len(p.Payload) < 5 {
				continue
			}
			target := binary.BigEndian.Uint16(p.Payload[2:])
			ack := append([]byte{}, p.Payload[:4]...)
			ack = append(ack, 0x00)
			c.Write(doip.Packet{Type: doip.TypeDiagnosticPosAck, Payload: ack}.Encode())
			if target != s.Logical {
				continue
			}
			for _, a := range s.App.Handle(p.Payload[4:], TransportCAN, false) {
				out := make([]byte, 4, 4+len(a))
				binary.BigEndian.PutUint16(out, s.Logical)
				binary.BigEndian.PutUint16(out[2:], tester)
				c.Write(doip.Packet{Type: doip.TypeDiagnosticMessage, Payload: append(out, a...)}.Encode())
			}
		}
	}
}
