package server

import (
	"strconv"
	"strings"
)

func strconvFormat(v float64, digits int) string {
	return strconv.FormatFloat(v, 'f', digits, 64)
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
