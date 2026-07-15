package traffic

import "encoding/binary"

type Session struct {
	InstanceID string `json:"instance_id"`
	RunID      string `json:"run_id"`
	Ports      []int  `json:"ports"`
}

type Totals struct {
	RunID   string `json:"run_id"`
	RXBytes uint64 `json:"rx_bytes"`
	TXBytes uint64 `json:"tx_bytes"`
}

type Packet struct {
	Length  uint64
	SrcPort uint16
	DstPort uint16
}

type Observer interface {
	Observe(Packet)
}

func decodeFrame(frame []byte, packetLength uint64) (Packet, bool) {
	if len(frame) < 14 {
		return Packet{}, false
	}
	etherType := binary.BigEndian.Uint16(frame[12:14])
	offset := 14
	for etherType == 0x8100 || etherType == 0x88a8 {
		if len(frame) < offset+4 {
			return Packet{}, false
		}
		etherType = binary.BigEndian.Uint16(frame[offset+2 : offset+4])
		offset += 4
	}
	var protocol byte
	switch etherType {
	case 0x0800:
		if len(frame) < offset+20 || frame[offset]>>4 != 4 {
			return Packet{}, false
		}
		headerLength := int(frame[offset]&0x0f) * 4
		if headerLength < 20 || len(frame) < offset+headerLength {
			return Packet{}, false
		}
		if binary.BigEndian.Uint16(frame[offset+6:offset+8])&0x1fff != 0 {
			return Packet{}, false
		}
		protocol = frame[offset+9]
		offset += headerLength
	case 0x86dd:
		if len(frame) < offset+40 || frame[offset]>>4 != 6 {
			return Packet{}, false
		}
		protocol = frame[offset+6]
		offset += 40
		var ok bool
		protocol, offset, ok = walkIPv6Extensions(frame, protocol, offset)
		if !ok {
			return Packet{}, false
		}
	default:
		return Packet{}, false
	}
	if protocol != 6 && protocol != 17 || len(frame) < offset+4 {
		return Packet{}, false
	}
	return Packet{
		Length:  packetLength,
		SrcPort: binary.BigEndian.Uint16(frame[offset : offset+2]),
		DstPort: binary.BigEndian.Uint16(frame[offset+2 : offset+4]),
	}, true
}

func walkIPv6Extensions(frame []byte, protocol byte, offset int) (byte, int, bool) {
	for {
		switch protocol {
		case 0, 43, 60:
			if len(frame) < offset+2 {
				return 0, 0, false
			}
			next, length := frame[offset], (int(frame[offset+1])+1)*8
			if len(frame) < offset+length {
				return 0, 0, false
			}
			protocol, offset = next, offset+length
		case 44:
			if len(frame) < offset+8 || binary.BigEndian.Uint16(frame[offset+2:offset+4])&0xfff8 != 0 {
				return 0, 0, false
			}
			protocol, offset = frame[offset], offset+8
		case 51:
			if len(frame) < offset+2 {
				return 0, 0, false
			}
			next, length := frame[offset], (int(frame[offset+1])+2)*4
			if len(frame) < offset+length {
				return 0, 0, false
			}
			protocol, offset = next, offset+length
		default:
			return protocol, offset, true
		}
	}
}
