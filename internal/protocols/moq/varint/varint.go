package varint

import (
	"fmt"
	"io"
)

type Varint uint64

func (v *Varint) Read(r io.Reader) error {
	var first [1]byte
	_, err := io.ReadFull(r, first[:])
	if err != nil {
		return err
	}
	b := first[0]

	var size int
	switch {
	case b&0x80 == 0:
		*v = Varint(b)
		return nil
	case b&0xC0 == 0x80:
		size = 2
	case b&0xE0 == 0xC0:
		size = 3
	case b&0xF0 == 0xE0:
		size = 4
	case b&0xF8 == 0xF0:
		size = 5
	case b&0xFC == 0xF8:
		size = 6
	default:
		return fmt.Errorf("unsupported varint prefix: 0x%02X", b)
	}

	rest := make([]byte, size-1)
	_, err = io.ReadFull(r, rest)
	if err != nil {
		return err
	}

	switch size {
	case 2:
		*v = Varint(b&0x3F)<<8 | Varint(rest[0])
	case 3:
		*v = Varint(b&0x1F)<<16 | Varint(rest[0])<<8 | Varint(rest[1])
	case 4:
		*v = Varint(b&0x0F)<<24 | Varint(rest[0])<<16 | Varint(rest[1])<<8 | Varint(rest[2])
	case 5:
		*v = Varint(b&0x07)<<32 | Varint(rest[0])<<24 | Varint(rest[1])<<16 | Varint(rest[2])<<8 | Varint(rest[3])
	default: // 6
		*v = Varint(b&0x03)<<40 | Varint(rest[0])<<32 | Varint(rest[1])<<24 | Varint(rest[2])<<16 | Varint(rest[3])<<8 | Varint(rest[4])
	}

	return nil
}

// Unmarshal decodes a leading-ones varint from buf and returns the number of bytes consumed.
func (v *Varint) Unmarshal(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, fmt.Errorf("not enough bytes")
	}
	b := buf[0]

	var size int
	switch {
	case b&0x80 == 0:
		*v = Varint(b)
		return 1, nil
	case b&0xC0 == 0x80:
		size = 2
	case b&0xE0 == 0xC0:
		size = 3
	case b&0xF0 == 0xE0:
		size = 4
	case b&0xF8 == 0xF0:
		size = 5
	case b&0xFC == 0xF8:
		size = 6
	default:
		return 0, fmt.Errorf("unsupported varint prefix: 0x%02X", b)
	}

	if len(buf) < size {
		return 0, fmt.Errorf("not enough bytes")
	}

	switch size {
	case 2:
		*v = Varint(b&0x3F)<<8 | Varint(buf[1])
	case 3:
		*v = Varint(b&0x1F)<<16 | Varint(buf[1])<<8 | Varint(buf[2])
	case 4:
		*v = Varint(b&0x0F)<<24 | Varint(buf[1])<<16 | Varint(buf[2])<<8 | Varint(buf[3])
	case 5:
		*v = Varint(b&0x07)<<32 | Varint(buf[1])<<24 | Varint(buf[2])<<16 | Varint(buf[3])<<8 | Varint(buf[4])
	default: // 6
		*v = Varint(b&0x03)<<40 | Varint(buf[1])<<32 | Varint(buf[2])<<24 | Varint(buf[3])<<16 | Varint(buf[4])<<8 | Varint(buf[5])
	}

	return size, nil
}

func (v Varint) MarshalSize() int {
	switch {
	case v < 1<<7:
		return 1
	case v < 1<<14:
		return 2
	case v < 1<<21:
		return 3
	case v < 1<<28:
		return 4
	case v < 1<<35:
		return 5
	default:
		return 6
	}
}

func (v Varint) MarshalTo(buf []byte) int {
	switch {
	case v < 1<<7:
		buf[0] = byte(v)
		return 1
	case v < 1<<14:
		buf[0] = 0x80 | byte(v>>8)
		buf[1] = byte(v)
		return 2
	case v < 1<<21:
		buf[0] = 0xC0 | byte(v>>16)
		buf[1] = byte(v >> 8)
		buf[2] = byte(v)
		return 3
	case v < 1<<28:
		buf[0] = 0xE0 | byte(v>>24)
		buf[1] = byte(v >> 16)
		buf[2] = byte(v >> 8)
		buf[3] = byte(v)
		return 4
	case v < 1<<35:
		buf[0] = 0xF0 | byte(v>>32)
		buf[1] = byte(v >> 24)
		buf[2] = byte(v >> 16)
		buf[3] = byte(v >> 8)
		buf[4] = byte(v)
		return 5
	default:
		buf[0] = 0xF8 | byte(v>>40)
		buf[1] = byte(v >> 32)
		buf[2] = byte(v >> 24)
		buf[3] = byte(v >> 16)
		buf[4] = byte(v >> 8)
		buf[5] = byte(v)
		return 6
	}
}

func (v Varint) Marshal() []byte {
	buf := make([]byte, v.MarshalSize())
	v.MarshalTo(buf)
	return buf
}
