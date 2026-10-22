package ecu

import (
	"context"
	"fmt"
	"strings"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// MatchKWP chooses the definition for a connected KWP2000 (or UDS)
// control unit by asking for its identification and measuring the
// length of its data records.
func (l *Library) MatchKWP(ctx context.Context, ch diag.Channel, protocol, profile string, log journal.Logger) *Definition {
	cands := l.ByProtocol(protocol, profile)
	if len(cands) == 0 {
		return nil
	}
	t := diag.DefaultTiming()
	// Identification text, if any candidate checks it.
	identText := ""
	for _, d := range cands {
		if len(d.Match.Ident) == 0 {
			continue
		}
		req := "1A 80"
		if len(d.Ident) > 0 {
			req = d.Ident[0].Request
		}
		if b, err := hexutil.Parse(req); err == nil {
			if resp, err := diag.Request(ctx, ch, b, t); err == nil {
				identText = strings.ToUpper(hexutil.ASCII(resp))
			}
		}
		break
	}
	// Record lengths.
	lens := map[string]int{}
	for _, d := range cands {
		for req := range d.Match.BlockLen {
			if _, done := lens[req]; done {
				continue
			}
			lens[req] = -1
			if b, err := hexutil.Parse(req); err == nil {
				if resp, err := diag.Request(ctx, ch, b, t); err == nil {
					lens[req] = len(resp)
				}
			}
		}
	}
	var best *Definition
	bestScore := 0
	for _, d := range cands {
		score := 0
		for req, n := range d.Match.BlockLen {
			if lens[req] == n {
				score += 10
			}
		}
		for _, s := range d.Match.Ident {
			if identText != "" && strings.Contains(identText, strings.ToUpper(s)) {
				score += 5
				break
			}
		}
		if score == 0 && d.Match.Fallback {
			score = 1
		}
		if score > bestScore {
			best, bestScore = d, score
		}
	}
	if best != nil {
		journal.Infof(log, "ecu", "control unit matched description %q (%s, score %d)", best.Name, best.ID, bestScore)
	}
	return best
}

// MatchLegacy chooses the definition for a legacy-protocol controller by
// its version byte.
func (l *Library) MatchLegacy(version byte) *Definition {
	v := fmt.Sprintf("%02X", version)
	var fallback *Definition
	for _, d := range l.ByProtocol("legacy", "") {
		for _, m := range d.Match.Version {
			if strings.EqualFold(m, v) {
				return d
			}
		}
		if d.Match.Fallback {
			fallback = d
		}
	}
	return fallback
}
