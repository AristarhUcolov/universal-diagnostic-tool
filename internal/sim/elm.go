package sim

import (
	"context"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/elm327"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/isotp"
)

// ELMBackend connects the ELM327 emulator to the simulated CAN car, so
// that the real ELM327 driver can be exercised without hardware.
type ELMBackend struct {
	mu     sync.Mutex
	mux    *can.Mux
	engine *Engine
	conns  map[uint32]*isotp.Conn
}

// NewELMBackend attaches to the bus.
func NewELMBackend(bus *CANBus, e *Engine) *ELMBackend {
	return &ELMBackend{mux: can.NewMux(bus.Endpoint("elm327"), nil), engine: e, conns: map[uint32]*isotp.Conn{}}
}

func (b *ELMBackend) conn(header []byte) *isotp.Conn {
	tx := uint32(0x7DF)
	if len(header) > 0 {
		tx = 0
		for _, v := range header {
			tx = tx<<8 | uint32(v)
		}
	}
	if c, ok := b.conns[tx]; ok {
		return c
	}
	var c *isotp.Conn
	if tx == 0x7DF {
		c = isotp.New(b.mux, isotp.OBDFunctional(false), nil)
	} else {
		c = isotp.New(b.mux, isotp.DefaultConfig(tx, tx+8), nil)
	}
	b.conns[tx] = c
	return c
}

// Transact implements elm327.Backend.
func (b *ELMBackend) Transact(ctx context.Context, proto byte, header []byte, data []byte) ([]elm327.Answer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.conn(header)
	msgs, err := diag.RequestAll(ctx, c, data, 500*time.Millisecond, 80*time.Millisecond)
	if err != nil {
		return nil, err
	}
	out := make([]elm327.Answer, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, elm327.Answer{Src: m.Src, Data: m.Data})
	}
	return out, nil
}

// Protocol implements elm327.Backend.
func (b *ELMBackend) Protocol() byte { return '6' }

// Voltage implements elm327.Backend.
func (b *ELMBackend) Voltage() float64 { return b.engine.State().BatteryV }

// ELMPort returns a port connected to an emulated ELM327 that is plugged
// into the simulated car.
func ELMPort(bus *CANBus, e *Engine) elm327.Port {
	client, server := bufferedPipe()
	em := elm327.NewEmulator(NewELMBackend(bus, e))
	em.Desc = "UDT simulated ELM327"
	go func() {
		_ = em.Serve(context.Background(), server)
		server.Close()
	}()
	return elm327.NewConnPort(client, "simulated ELM327")
}
