package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeSubscribe varint.Varint = 0x03

// Subscribe is SUBSCRIBE (0x03).
type Subscribe struct {
	RequestID uint64
	Namespace []string
	TrackName string
}

func (*Subscribe) isMessage() {}

func (m *Subscribe) unmarshal(buf []byte) error {
	var requestID varint.Varint
	n, err := requestID.Unmarshal(buf)
	if err != nil {
		return err
	}
	m.RequestID = uint64(requestID)
	buf = buf[n:]

	var nsCount varint.Varint
	n, err = nsCount.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	m.Namespace = make([]string, nsCount)
	for i := range m.Namespace {
		var l varint.Varint
		n, err = l.Unmarshal(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
		if len(buf) < int(l) {
			return fmt.Errorf("not enough bytes for namespace part")
		}
		m.Namespace[i] = string(buf[:l])
		buf = buf[int(l):]
	}

	var tnLen varint.Varint
	n, err = tnLen.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]
	if len(buf) < int(tnLen) {
		return fmt.Errorf("not enough bytes for track name")
	}
	m.TrackName = string(buf[:tnLen])

	// ignore parameters
	return nil
}

func (m Subscribe) marshalSize() int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		varint.Varint(len(m.Namespace)).MarshalSize()
	for _, part := range m.Namespace {
		payloadSize += varint.Varint(len(part)).MarshalSize() + len(part)
	}
	payloadSize += varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName)
	payloadSize++ // empty parameters count
	return typeSubscribe.MarshalSize() + 2 + payloadSize
}

func (m Subscribe) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		varint.Varint(len(m.Namespace)).MarshalSize()
	for _, part := range m.Namespace {
		payloadSize += varint.Varint(len(part)).MarshalSize() + len(part)
	}
	payloadSize += varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName)
	payloadSize++ // empty parameters count
	pos := typeSubscribe.MarshalTo(buf)
	buf[pos] = byte(payloadSize >> 8)
	buf[pos+1] = byte(payloadSize)
	pos += 2
	pos += varint.Varint(m.RequestID).MarshalTo(buf[pos:])
	pos += varint.Varint(len(m.Namespace)).MarshalTo(buf[pos:])
	for _, part := range m.Namespace {
		pos += varint.Varint(len(part)).MarshalTo(buf[pos:])
		pos += copy(buf[pos:], part)
	}
	pos += varint.Varint(len(m.TrackName)).MarshalTo(buf[pos:])
	pos += copy(buf[pos:], m.TrackName)
	buf[pos] = 0x00 // parameters count = 0
	return pos + 1
}

func (m Subscribe) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
