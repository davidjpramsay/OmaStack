package proxy

import (
	"net"
	"sync"
)

// A generation owns accepted and upstream connections, including hijacked
// WebSockets that net/http's shutdown bookkeeping deliberately stops tracking.
type connectionSet struct {
	mu          sync.Mutex
	closed      bool
	connections map[*trackedConn]struct{}
}

func newConnectionSet() *connectionSet {
	return &connectionSet{connections: make(map[*trackedConn]struct{})}
}

func (s *connectionSet) track(conn net.Conn) (net.Conn, error) {
	wrapper := &trackedConn{Conn: conn, owner: s}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	s.connections[wrapper] = struct{}{}
	s.mu.Unlock()
	return wrapper, nil
}

func (s *connectionSet) closeAll() {
	s.mu.Lock()
	s.closed = true
	connections := make([]*trackedConn, 0, len(s.connections))
	for conn := range s.connections {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

type trackedConn struct {
	net.Conn
	owner *connectionSet
	once  sync.Once
	err   error
}

func (c *trackedConn) Close() error {
	c.once.Do(func() {
		c.err = c.Conn.Close()
		c.owner.mu.Lock()
		delete(c.owner.connections, c)
		c.owner.mu.Unlock()
	})
	return c.err
}

type trackedListener struct {
	net.Listener
	connections *connectionSet
}

func (l *trackedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return l.connections.track(conn)
}
