package moq

import (
	"context"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/quic-go/webtransport-go"
)

// mpeg4AudioSamplesPerFrame is the number of samples per AAC-LC access unit.
const mpeg4AudioSamplesPerFrame = 1024

func sendSubgroup(wt *webtransport.Session, trackAlias uint64, pts uint64, payload []byte) error {
	uniStream, err := wt.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer uniStream.Close() //nolint:errcheck

	var buf []byte
	buf = append(buf, subgroup.Header{TrackAlias: trackAlias, GroupID: pts}.Marshal()...)
	buf = append(buf, subgroup.Object{IDDelta: 0, Payload: payload}.Marshal()...)
	buf = append(buf, subgroup.End{}.Marshal()...)

	_, err = uniStream.Write(buf)
	return err
}

// FromStream maps a MediaMTX stream to a Media-over-QUIC stream.
func FromStream(
	media *description.Media,
	forma format.Format,
	r *stream.Reader,
	wt *webtransport.Session,
	requestID uint64,
) {
	switch forma.(type) {
	case *format.AV1:
		firstRandomAccess := false

		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			if !firstRandomAccess && !av1.IsRandomAccess2(u.Payload.(unit.PayloadAV1)) {
				return nil
			}
			firstRandomAccess = true

			payload, err := av1.Bitstream([][]byte(u.Payload.(unit.PayloadAV1))).Marshal()
			if err != nil {
				return err
			}

			return sendSubgroup(wt, requestID, uint64(u.PTS), payload)
		})

	case *format.VP9:
		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			return sendSubgroup(wt, requestID, uint64(u.PTS), u.Payload.(unit.PayloadVP9))
		})

	case *format.VP8:
		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			return sendSubgroup(wt, requestID, uint64(u.PTS), u.Payload.(unit.PayloadVP8))
		})

	case *format.H265:
		firstRandomAccess := false

		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			if !firstRandomAccess && !h264.IsRandomAccess(u.Payload.(unit.PayloadH265)) {
				return nil
			}
			firstRandomAccess = true

			payload, err := h264.AVCC([][]byte(u.Payload.(unit.PayloadH265))).Marshal()
			if err != nil {
				return err
			}

			return sendSubgroup(wt, requestID, uint64(u.PTS), payload)
		})

	case *format.H264:
		firstRandomAccess := false

		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			if !firstRandomAccess && !h264.IsRandomAccess(u.Payload.(unit.PayloadH264)) {
				return nil
			}
			firstRandomAccess = true

			payload, err := h264.AVCC([][]byte(u.Payload.(unit.PayloadH264))).Marshal()
			if err != nil {
				return err
			}

			return sendSubgroup(wt, requestID, uint64(u.PTS), payload)
		})

	case *format.Opus:
		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			pts := u.PTS

			for _, pkt := range u.Payload.(unit.PayloadOpus) {
				err := sendSubgroup(wt, requestID, uint64(u.PTS), pkt)
				if err != nil {
					return err
				}

				pts += opus.PacketDuration2(pkt)
			}

			return nil
		})

	case *format.MPEG4Audio:
		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			for i, au := range u.Payload.(unit.PayloadMPEG4Audio) {
				pts := uint64(u.PTS) + uint64(i)*mpeg4AudioSamplesPerFrame

				err := sendSubgroup(wt, requestID, pts, au)
				if err != nil {
					return err
				}
			}
			return nil
		})

	case *format.G711:
		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			return sendSubgroup(wt, requestID, uint64(u.PTS), u.Payload.(unit.PayloadG711))
		})

	case *format.LPCM:
		r.OnData(media, forma, func(u *unit.Unit) error {
			if u.NilPayload() {
				return nil
			}

			return sendSubgroup(wt, requestID, uint64(u.PTS), u.Payload.(unit.PayloadLPCM))
		})
	}
}
