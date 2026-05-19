package moq

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/stream"
	mediastream "github.com/bluenviron/mediamtx/internal/stream"
	"github.com/google/uuid"
	"github.com/quic-go/webtransport-go"
	"golang.org/x/sync/errgroup"
)

func findFormatByIndex(medias []*description.Media, idx int) (*description.Media, format.Format, bool) {
	i := 0
	for _, media := range medias {
		for _, forma := range media.Formats {
			if i == idx {
				return media, forma, true
			}
			i++
		}
	}
	return nil, nil, false
}

type session struct {
	wt          *webtransport.Session
	wg          *sync.WaitGroup
	pathName    string
	pathManager serverPathManager
	parent      logger.Writer

	ctx                context.Context
	ctxCancel          context.CancelFunc
	created            time.Time
	uuid               uuid.UUID
	mutex              sync.Mutex
	path               defs.Path
	stream             *stream.Stream
	trackSubscriptions map[int]struct{}

	recvSetupStreamOK chan struct{}
	done              chan struct{}
}

func (s *session) initialize() {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.created = time.Now()
	s.uuid = uuid.New()
	s.trackSubscriptions = make(map[int]struct{})

	s.recvSetupStreamOK = make(chan struct{})
	s.done = make(chan struct{})

	s.Log(logger.Info, "created by %s", s.wt.RemoteAddr())

	s.wg.Add(1)
	go s.run()
}

func (s *session) apiItem() defs.APIMOQSession {
	return defs.APIMOQSession{
		ID:         s.uuid,
		Created:    s.created,
		RemoteAddr: s.wt.RemoteAddr().String(),
		Path:       s.pathName,
	}
}

// Close implements defs.Reader.
func (s *session) Close() {
	s.ctxCancel()
}

// APIReaderDescribe implements defs.Reader.
func (s *session) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeMOQSession,
		ID:   s.uuid.String(),
	}
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %v] "+format, append([]any{id}, args...)...)
}

func (s *session) run() {
	defer s.wg.Done()
	defer close(s.done)

	err := s.runInner()

	if s.path != nil {
		s.path.RemoveReader(defs.PathRemoveReaderReq{Author: s})
	}

	s.Log(logger.Info, "closed: %v", err)
}

func (s *session) runInner() error {
	errGroup, errGroupCtx := errgroup.WithContext(context.Background())

	errGroup.Go(func() error {
		return s.runUniStreamAcceptor(errGroup)
	})

	errGroup.Go(func() error {
		return s.runBidirectionalStreamAcceptor(errGroup)
	})

	errGroup.Go(func() error {
		return s.runSetupWriter()
	})

	select {
	case <-s.ctx.Done():
		s.wt.CloseWithError(0, "") //nolint:errcheck
		errGroup.Wait()
		return fmt.Errorf("terminated")

	case <-errGroupCtx.Done():
		s.ctxCancel()
		s.wt.CloseWithError(0, "") //nolint:errcheck
		return errGroup.Wait()
	}
}

func (s *session) runUniStreamAcceptor(errGroup *errgroup.Group) error {
	for {
		stream, err := s.wt.AcceptUniStream(context.Background())
		if err != nil {
			return fmt.Errorf("AcceptUniStream returned: %w", err)
		}

		errGroup.Go(func() error {
			return s.runUniStream(stream)
		})
	}
}

func (s *session) runUniStream(stream *webtransport.ReceiveStream) error {
	msg, err := controlmessage.Read(stream)
	if err != nil {
		return err
	}

	switch msg.(type) {
	case *controlmessage.Setup:
		err := func() error {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			select {
			case <-s.recvSetupStreamOK:
				return fmt.Errorf("SETUP stream is already present")
			default:
				close(s.recvSetupStreamOK)
				return nil
			}
		}()
		if err != nil {
			return err
		}

		// TODO: check capabilities

		io.Copy(io.Discard, stream) //nolint:errcheck
		return fmt.Errorf("SETUP stream closed")

	default:
		return fmt.Errorf("unsupported stream type: %T", msg)
	}
}

func (s *session) runBidirectionalStreamAcceptor(errGroup *errgroup.Group) error {
	for {
		stream, err := s.wt.AcceptStream(context.Background())
		if err != nil {
			return fmt.Errorf("AcceptStream returned: %w", err)
		}

		errGroup.Go(func() error {
			return s.runBidirectionalStream(stream)
		})
	}
}

func (s *session) runBidirectionalStream(stream *webtransport.Stream) error {
	select {
	case <-s.recvSetupStreamOK:
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	msg, err := controlmessage.Read(stream)
	if err != nil {
		return err
	}

	switch m := msg.(type) {
	case *controlmessage.Subscribe:
		s.Log(logger.Info, "SUBSCRIBE namespace=%v track=%s", m.Namespace, m.TrackName)

		if m.TrackName == ".catalog" {
			return s.runCatalogSubscription(stream, m)
		}

		return s.runTrackSubscription(stream, m)

	default:
		return fmt.Errorf("unsupported message type: %T", msg)
	}
}

func (s *session) runCatalogSubscription(wtStream *webtransport.Stream, m *controlmessage.Subscribe) error {
	descRes := s.pathManager.Describe(defs.PathDescribeReq{
		AccessRequest: defs.PathAccessRequest{
			Name:     s.pathName,
			SkipAuth: true,
		},
	})
	if descRes.Err != nil {
		return descRes.Err
	}

	if _, err := wtStream.Write(controlmessage.SubscribeOk{TrackAlias: m.RequestID}.Marshal()); err != nil {
		return err
	}

	var cat catalog.Catalog
	cat.FromStream(descRes.Stream)
	catalogData, err := json.Marshal(cat)
	if err != nil {
		return err
	}

	dataStream, err := s.wt.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}

	var buf []byte
	buf = append(buf, subgroup.Header{TrackAlias: m.RequestID, GroupID: 0}.Marshal()...)
	buf = append(buf, subgroup.Object{IDDelta: 0, Payload: catalogData}.Marshal()...)
	buf = append(buf, subgroup.End{}.Marshal()...)
	if _, err := dataStream.Write(buf); err != nil {
		return err
	}

	io.Copy(io.Discard, wtStream) //nolint:errcheck
	return fmt.Errorf("SUBSCRIBE stream closed")
}

func (s *session) runTrackSubscription(wtStream *webtransport.Stream, m *controlmessage.Subscribe) error {
	trackID, err := strconv.Atoi(m.TrackName)
	if err != nil || trackID < 0 {
		return fmt.Errorf("invalid track name: %s", m.TrackName)
	}

	var media *description.Media
	var forma format.Format

	err = func() error {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		if len(s.trackSubscriptions) == 0 {
			addRes, err := s.pathManager.AddReader(defs.PathAddReaderReq{
				Author: s,
				AccessRequest: defs.PathAccessRequest{
					Name:     s.pathName,
					SkipAuth: true,
				},
			})
			if err != nil {
				return err
			}

			s.path = addRes.Path
			s.stream = addRes.Stream

			s.Log(logger.Info, "is reading from path %s", s.pathName)
		} else {
			_, ok := s.trackSubscriptions[trackID]
			if ok {
				return fmt.Errorf("already subscribed to track %d", trackID)
			}
		}

		var ok bool
		media, forma, ok = findFormatByIndex(s.stream.Desc.Medias, trackID)
		if !ok {
			return fmt.Errorf("track index %d out of range", trackID)
		}

		s.trackSubscriptions[trackID] = struct{}{}

		return nil
	}()
	if err != nil {
		return err
	}

	r := &mediastream.Reader{Parent: s}

	moq.FromStream(
		media,
		forma,
		r,
		s.wt,
		m.RequestID)

	s.stream.AddReader(r)
	defer s.stream.RemoveReader(r)

	_, err = wtStream.Write(controlmessage.SubscribeOk{TrackAlias: m.RequestID}.Marshal())
	if err != nil {
		return err
	}

	streamDone := make(chan struct{})
	go func() {
		io.Copy(io.Discard, wtStream) //nolint:errcheck
		close(streamDone)
	}()

	select {
	case err := <-r.Error():
		return err
	case <-streamDone:
		return fmt.Errorf("SUBSCRIBE stream closed")
		// case <-s.ctx.Done():
		//	return fmt.Errorf("terminated")
	}
}

func (s *session) runSetupWriter() error {
	sendSetupStream, err := s.wt.OpenUniStreamSync(context.Background())
	if err != nil {
		return err
	}

	if _, err := sendSetupStream.Write(controlmessage.Setup{}.Marshal()); err != nil {
		return err
	}

	return nil
}
