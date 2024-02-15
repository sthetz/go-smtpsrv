package smtpsrv

import (
	"github.com/emersion/go-smtp"
)

// The Backend implements SMTP server methods.
type Backend struct {
	handler HandlerFunc
	auther  AuthFunc
}

func NewBackend(auther AuthFunc, handler HandlerFunc) *Backend {
	return &Backend{
		handler: handler,
		auther:  auther,
	}
}

// NewSession creates a new session for the given connection.
func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return NewSession(c, bkd.handler), nil
}
