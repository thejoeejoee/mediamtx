package catalog

import (
	"bytes"
	"encoding/base64"
	"strconv"

	"github.com/abema/go-mp4"
	codecsh264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	codecsh265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type Catalog struct {
	Version int            `json:"version"`
	Tracks  []CatalogTrack `json:"tracks"`
}

// FromStream populates the catalog from a live stream description.
func (c *Catalog) FromStream(strm *stream.Stream) error {
	var tracks []CatalogTrack
	i := 0

	for _, media := range strm.Desc.Medias {
		for _, forma := range media.Formats {
			var ct CatalogTrack
			err := ct.fromFormat(i, forma)
			if err != nil {
				return err
			}
			tracks = append(tracks, ct)
			i++
		}
	}

	c.Version = 1
	c.Tracks = tracks

	return nil
}

type CatalogTrack struct {
	Name       string  `json:"name"`
	Packaging  string  `json:"packaging"`
	IsLive     bool    `json:"isLive"`
	Namespace  string  `json:"namespace,omitempty"`
	Codec      string  `json:"codec,omitempty"`
	Bitrate    int     `json:"bitrate,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Framerate  float64 `json:"framerate,omitempty"`
	Samplerate int     `json:"samplerate,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	ClockRate  int     `json:"clockrate,omitempty"`
	InitData   string  `json:"initData,omitempty"`
}

func (ct *CatalogTrack) fromFormat(idx int, forma format.Format) error {
	ct.Name = strconv.Itoa(idx)
	ct.Packaging = "loc"
	ct.IsLive = true
	ct.ClockRate = forma.ClockRate()

	switch f := forma.(type) {
	case *format.H264:
		// TODO: fill better
		ct.Codec = "avc1.640028"

		sps, pps := f.SafeParams()
		if sps != nil && pps != nil {
			var psps codecsh264.SPS
			err := psps.Unmarshal(sps)
			if err != nil {
				return err
			}

			ct.Width = psps.Width()
			ct.Height = psps.Height()

			avcc := &mp4.AVCDecoderConfiguration{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeAvcC(),
				},
				ConfigurationVersion:       1,
				Profile:                    psps.ProfileIdc,
				ProfileCompatibility:       sps[2],
				Level:                      psps.LevelIdc,
				Reserved:                   0b111111,
				LengthSizeMinusOne:         3,
				Reserved2:                  0b111,
				NumOfSequenceParameterSets: 1,
				SequenceParameterSets: []mp4.AVCParameterSet{
					{
						Length:  uint16(len(sps)),
						NALUnit: sps,
					},
				},
				NumOfPictureParameterSets: 1,
				PictureParameterSets: []mp4.AVCParameterSet{
					{
						Length:  uint16(len(pps)),
						NALUnit: pps,
					},
				},
			}

			var buf bytes.Buffer
			_, err = mp4.Marshal(&buf, avcc, mp4.Context{})
			if err != nil {
				return err
			}
			ct.InitData = base64.StdEncoding.EncodeToString(buf.Bytes())
		}

	case *format.H265:
		// TODO: fill better
		ct.Codec = "hvc1.1.6.L93.B0"

		vps, sps, pps := f.SafeParams()
		if vps != nil && sps != nil && pps != nil {
			var psps codecsh265.SPS
			err := psps.Unmarshal(sps)
			if err != nil {
				return err
			}

			ct.Width = psps.Width()
			ct.Height = psps.Height()

			hvcc := &mp4.HvcC{
				ConfigurationVersion:        1,
				GeneralProfileIdc:           psps.ProfileTierLevel.GeneralProfileIdc,
				GeneralProfileCompatibility: psps.ProfileTierLevel.GeneralProfileCompatibilityFlag,
				GeneralConstraintIndicator: [6]uint8{
					sps[7], sps[8], sps[9],
					sps[10], sps[11], sps[12],
				},
				GeneralLevelIdc:      psps.ProfileTierLevel.GeneralLevelIdc,
				Reserved1:            0b1111,
				Reserved2:            0b111111,
				Reserved3:            0b111111,
				ChromaFormatIdc:      uint8(psps.ChromaFormatIdc),
				Reserved4:            0b11111,
				BitDepthLumaMinus8:   uint8(psps.BitDepthLumaMinus8),
				Reserved5:            0b11111,
				BitDepthChromaMinus8: uint8(psps.BitDepthChromaMinus8),
				NumTemporalLayers:    1,
				LengthSizeMinusOne:   3,
				NumOfNaluArrays:      3,
				NaluArrays: []mp4.HEVCNaluArray{
					{
						NaluType: byte(codecsh265.NALUType_VPS_NUT),
						NumNalus: 1,
						Nalus: []mp4.HEVCNalu{{
							Length:  uint16(len(vps)),
							NALUnit: vps,
						}},
					},
					{
						NaluType: byte(codecsh265.NALUType_SPS_NUT),
						NumNalus: 1,
						Nalus: []mp4.HEVCNalu{{
							Length:  uint16(len(sps)),
							NALUnit: sps,
						}},
					},
					{
						NaluType: byte(codecsh265.NALUType_PPS_NUT),
						NumNalus: 1,
						Nalus: []mp4.HEVCNalu{{
							Length:  uint16(len(pps)),
							NALUnit: pps,
						}},
					},
				},
			}

			var buf bytes.Buffer
			_, err = mp4.Marshal(&buf, hvcc, mp4.Context{})
			if err != nil {
				return err
			}
			ct.InitData = base64.StdEncoding.EncodeToString(buf.Bytes())
		}

	case *format.AV1:
		// TODO: fill better
		ct.Codec = "av01.0.04M.08"

	case *format.VP9:
		// TODO: fill better
		ct.Codec = "vp09.00.10.08"

	case *format.VP8:
		ct.Codec = "vp8"

	case *format.Opus:
		ct.Codec = "opus"
		ct.Samplerate = 48000
		ct.Channels = f.ChannelCount

	case *format.MPEG4Audio:
		// TODO: fill better
		ct.Codec = "mp4a.40.2"

		if f.Config != nil {
			ct.Samplerate = f.Config.SampleRate
			ct.Channels = int(f.Config.ChannelConfig)

			enc, err := f.Config.Marshal()
			if err != nil {
				return err
			}

			ct.InitData = base64.StdEncoding.EncodeToString(enc)
		}

	case *format.AC3:
		ct.Codec = "ac-3"
		ct.Samplerate = f.SampleRate
		ct.Channels = f.ChannelCount

	case *format.G711:
		if f.MULaw {
			ct.Codec = "mulaw"
		} else {
			ct.Codec = "alaw"
		}
		ct.Samplerate = f.SampleRate

	case *format.LPCM:
		ct.Codec = "pcm"
		ct.Samplerate = f.SampleRate
		ct.Channels = f.ChannelCount
	}

	return nil
}
