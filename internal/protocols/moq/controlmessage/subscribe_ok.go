package controlmessage

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

const typeSubscribeOk varint.Varint = 0x04

// SubscribeOk is SUBSCRIBE_OK (0x04).
type SubscribeOk struct {
	TrackAlias uint64
}

func (*SubscribeOk) isMessage() {}

func (m *SubscribeOk) unmarshal(buf []byte) error {
	var v varint.Varint
	_, err := v.Unmarshal(buf)
	if err != nil {
		return err
	}
	m.TrackAlias = uint64(v)
	return nil
}

func (m SubscribeOk) marshalSize() int {
	payloadSize := varint.Varint(m.TrackAlias).MarshalSize() + 1
	return typeSubscribeOk.MarshalSize() + 2 + payloadSize
}

func (m SubscribeOk) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(m.TrackAlias).MarshalSize() + 1
	pos := typeSubscribeOk.MarshalTo(buf)
	buf[pos] = byte(payloadSize >> 8)
	buf[pos+1] = byte(payloadSize)
	pos += 2
	pos += varint.Varint(m.TrackAlias).MarshalTo(buf[pos:])
	buf[pos] = 0x00
	return pos + 1
}

func (m SubscribeOk) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
