package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIMOQServer contains methods used by the API server.
type APIMOQServer interface {
	APISessionsList() (*APIMOQSessionList, error)
	APISessionsGet(uuid.UUID) (*APIMOQSession, error)
}

// APIMOQSessionList is a list of MOQ sessions.
type APIMOQSessionList struct {
	ItemCount int             `json:"itemCount"`
	PageCount int             `json:"pageCount"`
	Items     []APIMOQSession `json:"items"`
}

// APIMOQSession is a MOQ session.
type APIMOQSession struct {
	ID         uuid.UUID `json:"id"`
	Created    time.Time `json:"created"`
	RemoteAddr string    `json:"remoteAddr"`
	Path       string    `json:"path"`
}
