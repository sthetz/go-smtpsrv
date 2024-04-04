package smtpsrv

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/emersion/go-smtp"
)

type ServerConfig struct {
	ListenAddr      string
	BannerDomain    string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	Handler         HandlerFunc
	Auther          AuthFunc
	MaxMessageBytes int64
	TLSConfig       *tls.Config
}

type Server struct {
	*smtp.Server
}

func NewServer(cfg *ServerConfig) *Server {
	s := smtp.NewServer(NewBackend(cfg.Auther, cfg.Handler))

	s.Addr = cfg.ListenAddr
	s.Domain = cfg.BannerDomain
	s.ReadTimeout = cfg.ReadTimeout
	s.WriteTimeout = cfg.WriteTimeout
	s.MaxMessageBytes = cfg.MaxMessageBytes
	s.AllowInsecureAuth = true
	s.AuthDisabled = true
	s.EnableSMTPUTF8 = false

	return &Server{s}
}

func NewServerTLS(cfg *ServerConfig) *Server {
	s := smtp.NewServer(NewBackend(cfg.Auther, cfg.Handler))

	s.Addr = cfg.ListenAddr
	s.Domain = cfg.BannerDomain
	s.ReadTimeout = cfg.ReadTimeout
	s.WriteTimeout = cfg.WriteTimeout
	s.MaxMessageBytes = cfg.MaxMessageBytes
	s.AllowInsecureAuth = true
	s.AuthDisabled = true
	s.EnableSMTPUTF8 = false
	s.EnableREQUIRETLS = true
	s.TLSConfig = cfg.TLSConfig

	return &Server{s}

}
func (s *Server) ListenAndServe() error {
	fmt.Println("⇨ smtp server started on", s.Addr)
	return s.Server.ListenAndServe()

}

func (s *Server) ListenAndServeTLS() error {
	fmt.Println("⇨ smtp server started on", s.Addr)
	return s.Server.ListenAndServeTLS()

}

func (s *Server) Close() error {
	fmt.Println("⇨ smtp server stopped")
	return s.Server.Close()
}
