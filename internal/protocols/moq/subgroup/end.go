package subgroup

// End is the terminal frame of a subgroup stream (id_delta=0, length=0, status=GROUP_END=3).
type End struct{}

func (End) marshalSize() int { return 3 }

func (End) marshalTo(buf []byte) int {
	buf[0] = 0x00
	buf[1] = 0x00
	buf[2] = 0x03
	return 3
}

func (End) Marshal() []byte {
	buf := make([]byte, End{}.marshalSize())
	End{}.marshalTo(buf)
	return buf
}
