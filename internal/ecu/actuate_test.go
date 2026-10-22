package ecu

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

// echoChannel answers every request positively and remembers it.
type echoChannel struct {
	mu   sync.Mutex
	sent []string
	resp chan []byte
}

func (c *echoChannel) Send(ctx context.Context, data []byte) error {
	c.mu.Lock()
	c.sent = append(c.sent, hexutil.Format(data))
	c.mu.Unlock()
	c.resp <- append([]byte{data[0] + 0x40}, data[1:]...)
	return nil
}

func (c *echoChannel) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	select {
	case d := <-c.resp:
		return diag.Message{Data: d, Time: time.Now()}, nil
	case <-time.After(timeout):
		return diag.Message{}, diag.ErrTimeout
	}
}

func (c *echoChannel) Info() diag.ChannelInfo { return diag.ChannelInfo{Protocol: "test"} }
func (c *echoChannel) Close() error           { return nil }

func (c *echoChannel) requests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sent...)
}

// A test with an engine condition is refused, not run, when the engine
// speed cannot be read.
func TestConditionUnknown(t *testing.T) {
	def := &Definition{ID: "t", Actuators: []ActDef{
		{ID: "inj", Kind: "pulse", On: "30 01 06 01", Off: "30 01 00", Condition: "engine_off"},
		{ID: "fan", Kind: "toggle", On: "30 0A 06 01", Off: "30 0A 00"},
	}}
	ch := &echoChannel{resp: make(chan []byte, 4)}
	h := NewDefHandler(ch, def, "kwp2000", "test")
	ctx := context.Background()
	_, err := h.Actuate(ctx, "inj", "pulse", 0)
	var ce *ConditionError
	if !errors.As(err, &ce) || !ce.Unknown || ce.Code() != "engine_unknown" {
		t.Fatalf("pulse without a known engine speed: %v", err)
	}
	if got := ch.requests(); len(got) != 0 {
		t.Fatalf("requests sent: %v", got)
	}
	// Tests without a condition do not need the engine speed.
	if _, err := h.Actuate(ctx, "fan", "on", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Actuate(ctx, "fan", "off", 0); err != nil {
		t.Fatal(err)
	}
}

// A timer that runs out just as its output is switched on again (it then
// waits for the lock the new "on" holds) must not end the new hold.
func TestStaleTimer(t *testing.T) {
	def := &Definition{ID: "t", Actuators: []ActDef{
		{ID: "fan", Kind: "toggle", On: "30 0A 06 01", Off: "30 0A 00"},
	}}
	ch := &echoChannel{resp: make(chan []byte, 4)}
	h := NewDefHandler(ch, def, "kwp2000", "test")
	ctx := context.Background()
	if _, err := h.Actuate(ctx, "fan", "on", 0); err != nil {
		t.Fatal(err)
	}
	h.amu.Lock()
	first := h.held["fan"]
	h.amu.Unlock()
	if _, err := h.Actuate(ctx, "fan", "on", 0); err != nil {
		t.Fatal(err)
	}
	h.release("fan", first) // what the first timer does once it gets the lock
	if !h.Actuators()[0].Active {
		t.Fatal("the first timer ended the new hold")
	}
	if got := ch.requests(); len(got) != 2 {
		t.Fatalf("requests %v, want two switch-ons and no hand-back", got)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestOutputTimeout checks that an output left on is handed back to the
// control unit by itself.
func TestOutputTimeout(t *testing.T) {
	def := &Definition{ID: "t", Actuators: []ActDef{
		{ID: "fan", Kind: "toggle", On: "30 0A 06 01", Off: "30 0A 00", TimeoutS: 1},
	}}
	ch := &echoChannel{resp: make(chan []byte, 4)}
	h := NewDefHandler(ch, def, "kwp2000", "test")
	ctx := context.Background()
	if _, err := h.Actuate(ctx, "fan", "on", 0); err != nil {
		t.Fatal(err)
	}
	if !h.Actuators()[0].Active {
		t.Fatal("fan not held after switching it on")
	}
	time.Sleep(1500 * time.Millisecond)
	if h.Actuators()[0].Active {
		t.Fatal("fan still held after its timeout")
	}
	got := ch.requests()
	if len(got) != 2 || got[1] != "30 0A 00" {
		t.Fatalf("requests %v, want the switch-on and the hand-back", got)
	}
	// Closing must not send the hand-back again.
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if n := len(ch.requests()); n != 2 {
		t.Fatalf("%d requests after closing", n)
	}
}
