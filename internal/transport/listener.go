package transport

import (
	"fmt"
	"io"
	"net"
)

// BindLocalhost binds 127.0.0.1:0 (kernel-assigned port) and returns
// the listener + selected port. Caller MUST Close() on shutdown.
func BindLocalhost() (net.Listener, int, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, fmt.Errorf("bind 127.0.0.1:0: %w", err)
	}
	addr, ok := lis.Addr().(*net.TCPAddr)
	if !ok {
		lis.Close()
		return nil, 0, fmt.Errorf("listener returned non-TCP addr: %T", lis.Addr())
	}
	return lis, addr.Port, nil
}

// WriteReady prints the supervisor handshake line "READY <port>\n" to w
// and flushes if w supports Sync. Caller MUST call AFTER BindLocalhost
// succeeds — never before.
func WriteReady(w io.Writer, port int) error {
	if _, err := fmt.Fprintf(w, "READY %d\n", port); err != nil {
		return err
	}
	if f, ok := w.(interface{ Sync() error }); ok {
		_ = f.Sync()
	}
	return nil
}
