package controlmessage

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

const typeSetup varint.Varint = 0x2F00

// Setup is the MoQ session establishment stream header (type 0x2F00).
type Setup struct{}

func (*Setup) isMessage() {}

func (*Setup) unmarshal(_ []byte) error { return nil }

func (Setup) marshalSize() int {
	return typeSetup.MarshalSize() + 2
}

func (Setup) marshalTo(buf []byte) int {
	pos := typeSetup.MarshalTo(buf)
	buf[pos] = 0x00
	buf[pos+1] = 0x00
	return pos + 2
}

func (Setup) Marshal() []byte {
	buf := make([]byte, Setup{}.marshalSize())
	Setup{}.marshalTo(buf)
	return buf
}
