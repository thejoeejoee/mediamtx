package subgroup

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

const streamType varint.Varint = 0x30

// Header is written once at the start of a subgroup unidirectional stream.
type Header struct {
	TrackAlias uint64
	GroupID    uint64
}

func (h Header) marshalSize() int {
	return streamType.MarshalSize() +
		varint.Varint(h.TrackAlias).MarshalSize() +
		varint.Varint(h.GroupID).MarshalSize()
}

func (h Header) marshalTo(buf []byte) int {
	pos := streamType.MarshalTo(buf)
	pos += varint.Varint(h.TrackAlias).MarshalTo(buf[pos:])
	pos += varint.Varint(h.GroupID).MarshalTo(buf[pos:])
	return pos
}

func (h Header) Marshal() []byte {
	buf := make([]byte, h.marshalSize())
	h.marshalTo(buf)
	return buf
}
