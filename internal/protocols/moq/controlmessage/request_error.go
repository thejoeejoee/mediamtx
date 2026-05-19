package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeRequestError varint.Varint = 0x05

// RequestError is REQUEST_ERROR (0x05).
type RequestError struct {
	Code   uint64
	Reason string
}

func (*RequestError) isMessage() {}

func (m *RequestError) unmarshal(buf []byte) error {
	var code varint.Varint
	n, err := code.Unmarshal(buf)
	if err != nil {
		return err
	}
	m.Code = uint64(code)
	buf = buf[n:]

	var retry varint.Varint
	n, err = retry.Unmarshal(buf) // retryInterval, ignored
	if err != nil {
		return err
	}
	buf = buf[n:]

	var l varint.Varint
	n, err = l.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]
	if len(buf) < int(l) {
		return fmt.Errorf("not enough bytes")
	}
	m.Reason = string(buf[:l])

	return nil
}

func (m RequestError) marshalSize() int {
	payloadSize := varint.Varint(m.Code).MarshalSize() + 1 +
		varint.Varint(len(m.Reason)).MarshalSize() + len(m.Reason)
	return typeRequestError.MarshalSize() + 2 + payloadSize
}

func (m RequestError) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(m.Code).MarshalSize() + 1 +
		varint.Varint(len(m.Reason)).MarshalSize() + len(m.Reason)
	pos := typeRequestError.MarshalTo(buf)
	buf[pos] = byte(payloadSize >> 8)
	buf[pos+1] = byte(payloadSize)
	pos += 2
	pos += varint.Varint(m.Code).MarshalTo(buf[pos:])
	buf[pos] = 0x00
	pos++
	pos += varint.Varint(len(m.Reason)).MarshalTo(buf[pos:])
	pos += copy(buf[pos:], m.Reason)
	return pos
}

func (m RequestError) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
