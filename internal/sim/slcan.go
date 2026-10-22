package sim

import (
	"bufio"
	"context"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/elm327"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/slcan"
)

// SLCANPort returns a port connected to an emulated Lawicel/CANable
// adapter plugged into the simulated bus.
func SLCANPort(bus *CANBus) slcan.Port {
	client, server := bufferedPipe()
	ep := bus.Endpoint("slcan")
	go func() {
		defer server.Close()
		open := false
		done := make(chan struct{})
		defer close(done)
		go func() {
			ctx := context.Background()
			for {
				select {
				case <-done:
					return
				default:
				}
				f, err := ep.Recv(ctx, 200*time.Millisecond)
				if err != nil || !open {
					continue
				}
				if _, err := server.Write([]byte(slcan.FormatFrame(f) + "\r")); err != nil {
					return
				}
			}
		}()
		r := bufio.NewReader(server)
		for {
			line, err := r.ReadString('\r')
			if err != nil {
				return
			}
			line = line[:len(line)-1]
			reply := "\r"
			switch {
			case line == "":
			case line == "V":
				reply = "V1013\r"
			case line == "C":
				open = false
			case line == "O" || line == "L":
				open = true
			case line[0] == 'S':
			case line[0] == 't' || line[0] == 'T':
				if f, ok := slcan.ParseFrame(line); ok {
					_ = ep.Send(context.Background(), f)
				}
				reply = "z\r"
				if line[0] == 'T' {
					reply = "Z\r"
				}
			default:
				reply = "\a"
			}
			if _, err := server.Write([]byte(reply)); err != nil {
				return
			}
		}
	}()
	return elm327.NewConnPort(client, "simulated CANable")
}
