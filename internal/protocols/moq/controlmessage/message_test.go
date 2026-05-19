package controlmessage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var cases = []struct {
	name string
	enc  []byte
	dec  Message
}{
	{
		name: "setup",
		enc: []byte{
			0xAF, 0x00, // type 0x2F00 (2-byte varint)
			0x00, 0x00, // length = 0
		},
		dec: &Setup{},
	},
	{
		name: "subscribe",
		enc: []byte{
			0x03,       // type 0x03
			0x00, 0x0B, // length = 11
			0x01,                   // RequestID = 1
			0x01,                   // namespace count = 1
			0x03, 0x66, 0x6F, 0x6F, // namespace[0] = "foo"
			0x03, 0x62, 0x61, 0x72, // track name = "bar"
			0x00, // parameters count = 0
		},
		dec: &Subscribe{
			RequestID: 1,
			Namespace: []string{"foo"},
			TrackName: "bar",
		},
	},
	{
		name: "subscribe_ok",
		enc: []byte{
			0x04,       // type 0x04
			0x00, 0x02, // length = 2
			0x01, // TrackAlias = 1
			0x00, // content type (ignored)
		},
		dec: &SubscribeOk{
			TrackAlias: 1,
		},
	},
	{
		name: "request_error",
		enc: []byte{
			0x05,       // type 0x05
			0x00, 0x06, // length = 6
			0x01,                   // Code = 1
			0x00,                   // retryInterval = 0 (ignored)
			0x03, 0x66, 0x6F, 0x6F, // Reason = "foo"
		},
		dec: &RequestError{
			Code:   1,
			Reason: "foo",
		},
	},
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			m, err := Read(bytes.NewReader(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.dec, m)
		})
	}
}

func TestMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			buf := ca.dec.Marshal()
			require.Equal(t, ca.enc, buf)
		})
	}
}
