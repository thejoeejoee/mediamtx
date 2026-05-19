package subgroup

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

// Object is one object frame within a subgroup stream.
type Object struct {
	IDDelta uint64
	Payload []byte
}

func (o Object) marshalSize() int {
	return varint.Varint(o.IDDelta).MarshalSize() +
		varint.Varint(len(o.Payload)).MarshalSize() +
		len(o.Payload)
}

func (o Object) marshalTo(buf []byte) int {
	pos := varint.Varint(o.IDDelta).MarshalTo(buf)
	pos += varint.Varint(len(o.Payload)).MarshalTo(buf[pos:])
	pos += copy(buf[pos:], o.Payload)
	return pos
}

func (o Object) Marshal() []byte {
	buf := make([]byte, o.marshalSize())
	o.marshalTo(buf)
	return buf
}
