package profiling

import (
	"log"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers
)

// Server wraps the profiling HTTP server
type Server struct {
	addr string
}

// NewServer creates a new profiling server
func NewServer(port string) *Server {
	if port == "" {
		port = "6060"
	}
	return &Server{addr: "localhost:" + port}
}

// Start starts the profiling server in a goroutine
func (s *Server) Start() {
	go func() {
		log.Printf("[profiling] starting pprof server on %s", s.addr)
		if err := http.ListenAndServe(s.addr, nil); err != nil {
			log.Printf("[profiling] server error: %v", err)
		}
	}()
}

// Addr returns the server address
func (s *Server) Addr() string {
	return s.addr
}
