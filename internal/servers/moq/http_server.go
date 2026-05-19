package moq

import (
	"crypto/sha256"
	"crypto/tls"
	_ "embed"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp3"
)

//go:embed publish_index.html
var publishIndex []byte

//go:embed read_index.html
var readIndex []byte

//go:embed reader.js
var readerJS []byte

// wtProtocolHeader is the response header that echoes the selected protocol.
// Its value is an SF String as required by the WebTransport spec.
const wtProtocolHeader = "WT-Protocol"

// wtAvailableProtocolsHeader is the request header carrying the client's supported protocols.
const wtAvailableProtocolsHeader = "WT-Available-Protocols"

// moqtVersion is the ALPN / WT-Available-Protocols token for MoQ Transport draft-18.
const moqtVersion = "moqt-18"

type ginUnwrapper interface {
	Unwrap() http.ResponseWriter
}

// containsMoqtVersion reports whether a WT-Available-Protocols header value
// includes moqtVersion.  It handles both SF Token (moqt-18) and SF String
// ("moqt-18") encodings and ignores per-item parameters.
func containsMoqtVersion(header string) bool {
	for _, item := range strings.Split(header, ",") {
		item = strings.TrimSpace(item)
		if i := strings.IndexByte(item, ';'); i >= 0 {
			item = item[:i]
		}
		item = strings.Trim(strings.TrimSpace(item), `"`)
		if item == moqtVersion {
			return true
		}
	}
	return false
}

func trailingSlashLocation(rawPath string, rawQuery string) string {
	res := path.Clean(rawPath)
	res = strings.TrimLeft(res, "/\\")
	res = "/" + res + "/"

	if rawQuery != "" {
		res += "?" + rawQuery
	}

	return res
}

func certFingerprint(cert *tls.Certificate) (string, error) {
	if len(cert.Certificate) == 0 {
		return "", fmt.Errorf("empty certificate")
	}
	sum := sha256.Sum256(cert.Certificate[0])
	return hex.EncodeToString(sum[:]), nil
}

type httpServerParent interface {
	newSession(req newSessionReq) newSessionRes
	logger.Writer
}

type httpServer struct {
	https2Address  string
	https3Address  string
	serverCert     string
	serverKey      string
	allowOrigins   []string
	trustedProxies conf.IPNetworks
	readTimeout    conf.Duration
	writeTimeout   conf.Duration
	parent         httpServerParent

	innerHTTPS2 *httpp.Server
	innerHTTPS3 *httpp3.Server
}

func (s *httpServer) initialize() error {
	routerHTTPS2 := gin.New()
	routerHTTPS2.SetTrustedProxies(s.trustedProxies.ToTrustedProxies()) //nolint:errcheck
	routerHTTPS2.Use(s.middlewarePreflightRequests)
	routerHTTPS2.Use(s.onRequestHTTPS2)

	s.innerHTTPS2 = &httpp.Server{
		Address:       s.https2Address,
		AllowOrigins:  s.allowOrigins,
		ReadTimeout:   time.Duration(s.readTimeout),
		WriteTimeout:  time.Duration(s.writeTimeout),
		Encryption:    true,
		ServerKey:     s.serverKey,
		ServerCert:    s.serverCert,
		AllowAutoCert: true,
		Handler:       routerHTTPS2,
		Parent:        s,
	}
	err := s.innerHTTPS2.Initialize()
	if err != nil {
		return err
	}

	routerHTTPS3 := gin.New()
	routerHTTPS3.Use(s.onRequestHTTPS3)

	s.innerHTTPS3 = &httpp3.Server{
		Address:            s.https3Address,
		Handler:            routerHTTPS3,
		EnableWebTransport: true,
		Parent:             s,
	}
	err = s.innerHTTPS3.Initialize()
	if err != nil {
		s.innerHTTPS2.Close()
		return err
	}

	return nil
}

// Log implements logger.Writer.
func (s *httpServer) Log(level logger.Level, format string, args ...any) {
	s.parent.Log(level, format, args...)
}

func (s *httpServer) close() {
	s.innerHTTPS3.Close()
	s.innerHTTPS2.Close()
}

func (s *httpServer) middlewarePreflightRequests(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (s *httpServer) writeErrorNoLog(ctx *gin.Context, status int, err error) {
	ctx.AbortWithStatusJSON(status, &defs.APIError{
		Status: defs.APIErrorStatusError,
		Error:  err.Error(),
	})
}

func (s *httpServer) onPage(ctx *gin.Context, pathName string, publish bool) {
	//if !s.checkAuthOutsideSession(ctx, pathName, publish) {
	//	return
	//}

	ctx.Header("Content-Type", "text/html")
	ctx.Writer.WriteHeader(http.StatusOK)

	if publish {
		ctx.Writer.Write(publishIndex)
	} else {
		ctx.Writer.Write(readIndex)
	}
}

func (s *httpServer) onFingerprint(ctx *gin.Context) {
	//if !s.checkAuthOutsideSession(ctx, pathName, publish) {
	//	return
	//}

	fp, err := certFingerprint(s.innerHTTPS3.Certificate())
	if err != nil {
		s.writeErrorNoLog(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.String(http.StatusOK, fp)
}

func (s *httpServer) onRequestHTTPS2(ctx *gin.Context) {
	// ctx.Writer.Write([]byte("OK\n"))

	/*_, http3Port, err := net.SplitHostPort(s.https3Address)
	if err != nil {
		s.ln.Close() //nolint:errcheck
		s.loader.Close()
		return fmt.Errorf("invalid HTTP/3 address: %w", err)
	}
	altSvc := `h3=":` + http3Port + `"; ma=86400`
	ctx.Header("Alt-Svc", altSvc)
	*/

	if ctx.Request.Method == http.MethodGet && strings.HasSuffix(ctx.Request.URL.Path, "/fingerprint") {
		s.onFingerprint(ctx)
		return
	}

	// static resources
	if ctx.Request.Method == http.MethodGet {
		switch {
		case strings.HasSuffix(ctx.Request.URL.Path, "/reader.js"):
			ctx.Header("Cache-Control", "max-age=3600")
			ctx.Header("Content-Type", "application/javascript")
			ctx.Writer.WriteHeader(http.StatusOK)
			ctx.Writer.Write(readerJS)

		case ctx.Request.URL.Path == "/favicon.ico":

		case len(ctx.Request.URL.Path) >= 2:
			switch {
			case len(ctx.Request.URL.Path) > len("/publish") && strings.HasSuffix(ctx.Request.URL.Path, "/publish"):
				s.onPage(ctx, ctx.Request.URL.Path[1:len(ctx.Request.URL.Path)-len("/publish")], true)

			case ctx.Request.URL.Path[len(ctx.Request.URL.Path)-1] != '/':
				ctx.Header("Location", trailingSlashLocation(ctx.Request.URL.Path, ctx.Request.URL.RawQuery))
				ctx.Writer.WriteHeader(http.StatusFound)

			default:
				s.onPage(ctx, ctx.Request.URL.Path[1:len(ctx.Request.URL.Path)-1], false)
			}
		}
		return
	}
}

func (s *httpServer) onRequestHTTPS3(ctx *gin.Context) {
	if ctx.Request.Method != http.MethodConnect || !strings.HasSuffix(ctx.Request.URL.Path, "/moq") || len(ctx.Request.URL.Path) <= len("/moq") {
		return
	}

	pathName := ctx.Request.URL.Path[1 : len(ctx.Request.URL.Path)-len("/moq")]
	if len(pathName) == 0 {
		return
	}

	// WT-Available-Protocols negotiation (MoQ Transport draft-18, section 3.1).
	// If the header is present, it must include moqtVersion; we then echo back
	// the selected version in WT-Protocol before the upgrade writes its 200.
	if offered := ctx.Request.Header.Get(wtAvailableProtocolsHeader); offered != "" {
		if !containsMoqtVersion(offered) {
			s.writeErrorNoLog(ctx, http.StatusBadRequest, fmt.Errorf("no supported MoQ version in %s", wtAvailableProtocolsHeader))
			return
		}
	}

	w := ctx.Writer.(ginUnwrapper).Unwrap()

	if offered := ctx.Request.Header.Get(wtAvailableProtocolsHeader); offered != "" {
		// SF String encoding: the field value includes the surrounding double-quotes.
		w.Header().Set(wtProtocolHeader, `"`+moqtVersion+`"`)
	}

	wt, err := s.innerHTTPS3.Upgrade(w, ctx.Request)
	if err != nil {
		s.writeErrorNoLog(ctx, http.StatusBadRequest, fmt.Errorf("webtransport upgrade failed: %w", err))
		return
	}

	res := s.parent.newSession(newSessionReq{
		pathName: pathName,
		wt:       wt,
	})
	if res.err != nil {
		wt.CloseWithError(0, res.err.Error()) //nolint:errcheck
		return
	}
}
