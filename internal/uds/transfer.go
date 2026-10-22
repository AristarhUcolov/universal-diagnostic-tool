package uds

import (
	"context"
	"fmt"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
)

// Progress reports transferred bytes.
type Progress func(done, total int)

// maxBlock reads maxNumberOfBlockLength from a 74/75 answer.
func maxBlock(resp []byte) (int, error) {
	if len(resp) < 3 {
		return 0, fmt.Errorf("uds: short transfer answer % X", resp)
	}
	n := int(resp[1] >> 4)
	if n == 0 || len(resp) < 2+n {
		return 0, fmt.Errorf("uds: bad length format in % X", resp)
	}
	v := 0
	for _, b := range resp[2 : 2+n] {
		v = v<<8 | int(b)
	}
	if v < 3 {
		return 0, fmt.Errorf("uds: block length %d too small", v)
	}
	return v, nil
}

func addrSize(addr, size uint32) []byte {
	return []byte{0x44, byte(addr >> 24), byte(addr >> 16), byte(addr >> 8), byte(addr),
		byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
}

// Download writes data to the control unit at addr (services 34, 36,
// 37). The caller must already be in the programming session with
// security access granted and the target area erased; doing this on a
// real vehicle without the manufacturer's procedure can brick the unit.
func (c *Client) Download(ctx context.Context, addr uint32, data []byte, progress Progress) error {
	resp, err := diag.Request(ctx, c.Ch, append([]byte{0x34, 0x00}, addrSize(addr, uint32(len(data)))...), c.Timing)
	if err != nil {
		return err
	}
	max, err := maxBlock(resp)
	if err != nil {
		return err
	}
	chunk := max - 2
	seq := byte(1)
	for off := 0; off < len(data); off += chunk {
		end := off + chunk
		if end > len(data) {
			end = len(data)
		}
		req := append([]byte{0x36, seq}, data[off:end]...)
		r, err := diag.Request(ctx, c.Ch, req, c.Timing)
		if err != nil {
			return fmt.Errorf("uds: block %d at offset %d: %w", seq, off, err)
		}
		if len(r) < 2 || r[1] != seq {
			return fmt.Errorf("uds: block counter mismatch (sent %d, answer % X)", seq, r)
		}
		seq++
		if progress != nil {
			progress(end, len(data))
		}
	}
	_, err = diag.Request(ctx, c.Ch, []byte{0x37}, c.Timing)
	return err
}

// Upload reads size bytes from addr (services 35, 36, 37). Many control
// units only allow this after security access.
func (c *Client) Upload(ctx context.Context, addr, size uint32, progress Progress) ([]byte, error) {
	resp, err := diag.Request(ctx, c.Ch, append([]byte{0x35, 0x00}, addrSize(addr, size)...), c.Timing)
	if err != nil {
		return nil, err
	}
	if _, err := maxBlock(resp); err != nil {
		return nil, err
	}
	out := make([]byte, 0, size)
	seq := byte(1)
	for uint32(len(out)) < size {
		r, err := diag.Request(ctx, c.Ch, []byte{0x36, seq}, c.Timing)
		if err != nil {
			return out, fmt.Errorf("uds: block %d: %w", seq, err)
		}
		if len(r) < 2 || r[1] != seq {
			return out, fmt.Errorf("uds: block counter mismatch (expected %d, answer % X)", seq, r)
		}
		if len(r) == 2 {
			break
		}
		out = append(out, r[2:]...)
		seq++
		if progress != nil {
			progress(len(out), int(size))
		}
	}
	if uint32(len(out)) > size {
		out = out[:size]
	}
	_, err = diag.Request(ctx, c.Ch, []byte{0x37}, c.Timing)
	return out, err
}
