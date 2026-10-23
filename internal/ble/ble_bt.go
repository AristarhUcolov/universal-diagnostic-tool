//go:build windows || linux

package ble

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// Available reports whether this build can use Bluetooth LE.
func Available() bool { return true }

var scanMu sync.Mutex // the adapter runs one scan at a time

// Scan lists the devices advertising during d, OBD adapters first.
func Scan(ctx context.Context, d time.Duration) ([]Device, error) {
	found, err := scan(ctx, d, "")
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(found))
	for _, dev := range found {
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Likely != out[j].Likely {
			return out[i].Likely
		}
		return out[i].RSSI > out[j].RSSI
	})
	return out, nil
}

// scan collects advertising devices for up to d; it ends early when the
// device with address want is seen.
func scan(ctx context.Context, d time.Duration, want string) (map[string]Device, error) {
	a := bluetooth.DefaultAdapter
	if err := enable(a); err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrOff, err)
	}
	scanMu.Lock()
	defer scanMu.Unlock()
	var mu sync.Mutex
	found := map[string]Device{}
	seen := make(chan struct{})
	var once sync.Once
	done := make(chan error, 1)
	go func() {
		done <- a.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
			addr := strings.ToUpper(r.Address.String())
			name := r.LocalName()
			mu.Lock()
			if old, ok := found[addr]; ok && name == "" {
				name = old.Name
			}
			found[addr] = Device{Address: addr, Name: name, RSSI: int(r.RSSI), Likely: likely(name)}
			mu.Unlock()
			if want != "" && strings.EqualFold(addr, want) {
				once.Do(func() { close(seen) })
			}
		})
	}()
	var err error
	select {
	case err = <-done: // the scan could not start
	case <-seen:
	case <-time.After(d):
	case <-ctx.Done():
	}
	if err == nil {
		_ = a.StopScan()
		select {
		case err = <-done:
		case <-time.After(3 * time.Second):
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if err != nil && len(found) == 0 {
		return nil, fmt.Errorf("%w (%v)", ErrOff, err)
	}
	return found, nil
}

// Open connects to the adapter at address ("AA:BB:CC:DD:EE:FF") and
// returns its serial line.
func Open(ctx context.Context, address string) (*Port, error) {
	address = strings.ToUpper(strings.TrimSpace(address))
	mac, err := bluetooth.ParseMAC(address)
	if err != nil {
		return nil, fmt.Errorf("ble: %q is not a Bluetooth address (find the adapter first)", address)
	}
	// The adapter must be advertising, and BlueZ connects only to a
	// device it has seen: look for it first.
	found, err := scan(ctx, 6*time.Second, address)
	if err != nil {
		return nil, err
	}
	dev, ok := found[address]
	if !ok {
		return nil, ErrNotFound
	}
	a := bluetooth.DefaultAdapter
	conn, err := a.Connect(bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("ble: cannot connect to %s: %w", address, err)
	}
	notify, write, err := findSerial(conn)
	if err != nil {
		conn.Disconnect()
		return nil, err
	}
	name := dev.Name
	if name == "" {
		name = address
	}
	p := newPort("ble://"+name, func(b []byte) error {
		if _, err := write.WriteWithoutResponse(b); err != nil {
			_, err = write.Write(b)
			return err
		}
		return nil
	}, func() error { return conn.Disconnect() })
	if err := notify.EnableNotifications(p.push); err != nil {
		conn.Disconnect()
		return nil, fmt.Errorf("ble: notifications: %w", err)
	}
	return p, nil
}

// findSerial finds the adapter's serial characteristics: a known pair
// first, then any service that notifies and takes writes.
func findSerial(conn bluetooth.Device) (notify, write bluetooth.DeviceCharacteristic, err error) {
	svcs, err := conn.DiscoverServices(nil)
	if err != nil {
		return notify, write, fmt.Errorf("ble: services: %w", err)
	}
	for _, pair := range knownPairs {
		for _, s := range svcs {
			if !strings.EqualFold(s.UUID().String(), pair.service) {
				continue
			}
			chars, err := s.DiscoverCharacteristics(nil)
			if err != nil {
				continue
			}
			var n, w = -1, -1
			for i := range chars {
				u := strings.ToLower(chars[i].UUID().String())
				if u == pair.notify {
					n = i
				}
				if u == pair.write {
					w = i
				}
			}
			if n >= 0 && w >= 0 {
				return chars[n], chars[w], nil
			}
		}
	}
	for _, s := range svcs {
		if standardService(s.UUID().String()) {
			continue
		}
		chars, err := s.DiscoverCharacteristics(nil)
		if err != nil {
			continue
		}
		if n, w, ok := byProperties(chars); ok {
			return n, w, nil
		}
	}
	return notify, write, ErrNoSerial
}

// standardService tells the services every device has (generic access,
// device information, battery…), which never carry the serial line.
func standardService(uuid string) bool {
	u := strings.ToLower(uuid)
	for _, s := range []string{"00001800-", "00001801-", "0000180a-", "0000180f-"} {
		if strings.HasPrefix(u, s) {
			return true
		}
	}
	return false
}
