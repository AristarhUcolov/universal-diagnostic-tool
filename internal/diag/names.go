package diag

import "fmt"

// NRCName returns the ISO 14229 / ISO 14230 name of a negative response
// code.
func NRCName(code byte) string {
	if s, ok := nrcNames[code]; ok {
		return s
	}
	switch {
	case code >= 0x38 && code <= 0x4F:
		return "reserved by extended data link security"
	case code >= 0x94 && code <= 0xFE:
		return "manufacturer specific condition not correct"
	}
	return fmt.Sprintf("unknown code 0x%02X", code)
}

var nrcNames = map[byte]string{
	0x10: "general reject",
	0x11: "service not supported",
	0x12: "sub-function not supported / invalid format",
	0x13: "incorrect message length or invalid format",
	0x14: "response too long",
	0x21: "busy, repeat request",
	0x22: "conditions not correct",
	0x23: "routine not complete",
	0x24: "request sequence error",
	0x25: "no response from sub-net component",
	0x26: "failure prevents execution of requested action",
	0x31: "request out of range",
	0x33: "security access denied",
	0x35: "invalid key",
	0x36: "exceeded number of attempts",
	0x37: "required time delay not expired",
	0x40: "download not accepted",
	0x41: "improper download type",
	0x42: "cannot download to specified address",
	0x43: "cannot download number of bytes requested",
	0x50: "upload not accepted",
	0x51: "improper upload type",
	0x52: "cannot upload from specified address",
	0x53: "cannot upload number of bytes requested",
	0x70: "upload/download not accepted",
	0x71: "transfer data suspended",
	0x72: "general programming failure",
	0x73: "wrong block sequence counter",
	0x74: "illegal address in block transfer",
	0x75: "illegal byte count in block transfer",
	0x76: "illegal block transfer type",
	0x77: "block transfer data checksum error",
	0x78: "request correctly received, response pending",
	0x79: "incorrect byte count during block transfer",
	0x7E: "sub-function not supported in active session",
	0x7F: "service not supported in active session",
	0x80: "service not supported in active diagnostic mode",
	0x81: "engine speed too high",
	0x82: "engine speed too low",
	0x83: "engine is running",
	0x84: "engine is not running",
	0x85: "engine run time too low",
	0x86: "temperature too high",
	0x87: "temperature too low",
	0x88: "vehicle speed too high",
	0x89: "vehicle speed too low",
	0x8A: "throttle/pedal too high",
	0x8B: "throttle/pedal too low",
	0x8C: "transmission range not in neutral",
	0x8D: "transmission range not in gear",
	0x8F: "brake switch(es) not closed",
	0x90: "shifter lever not in park",
	0x91: "torque converter clutch locked",
	0x92: "voltage too high",
	0x93: "voltage too low",
}

// ServiceName names a request service id (UDS, KWP2000 or OBD-II).
func ServiceName(sid byte) string {
	if s, ok := serviceNames[sid]; ok {
		return s
	}
	if sid&0x40 != 0 {
		if s, ok := serviceNames[sid&^0x40]; ok {
			return s + " (positive response)"
		}
	}
	return fmt.Sprintf("service 0x%02X", sid)
}

var serviceNames = map[byte]string{
	0x01: "OBD show current data",
	0x02: "OBD show freeze frame data",
	0x03: "OBD show stored DTCs",
	0x04: "OBD clear DTCs",
	0x05: "OBD oxygen sensor monitoring",
	0x06: "OBD on-board monitoring results",
	0x07: "OBD show pending DTCs",
	0x08: "OBD control on-board system",
	0x09: "OBD request vehicle information",
	0x0A: "OBD permanent DTCs",
	0x10: "diagnostic session control",
	0x11: "ECU reset",
	0x12: "read freeze frame data (KWP)",
	0x13: "read diagnostic trouble codes (KWP)",
	0x14: "clear diagnostic information",
	0x17: "read status of DTC (KWP)",
	0x18: "read DTC by status (KWP)",
	0x19: "read DTC information",
	0x1A: "read ECU identification (KWP)",
	0x20: "stop diagnostic session (KWP)",
	0x21: "read data by local identifier (KWP)",
	0x22: "read data by identifier",
	0x23: "read memory by address",
	0x24: "read scaling data by identifier",
	0x27: "security access",
	0x28: "communication control",
	0x29: "authentication",
	0x2A: "read data by periodic identifier",
	0x2C: "dynamically define data identifier",
	0x2E: "write data by identifier",
	0x2F: "input/output control by identifier",
	0x30: "input/output control by local identifier (KWP)",
	0x31: "routine control",
	0x32: "stop routine by local identifier (KWP)",
	0x33: "request routine results (KWP)",
	0x34: "request download",
	0x35: "request upload",
	0x36: "transfer data",
	0x37: "request transfer exit",
	0x38: "request file transfer",
	0x3B: "write data by local identifier (KWP)",
	0x3D: "write memory by address",
	0x3E: "tester present",
	0x81: "start communication (KWP)",
	0x82: "stop communication (KWP)",
	0x83: "access timing parameters (KWP)",
	0x84: "secured data transmission",
	0x85: "control DTC setting",
	0x86: "response on event",
	0x87: "link control",
}
