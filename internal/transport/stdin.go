// Package transport handles the supervisor handshake plumbing — stdin
// cert reading and READY-line announcement after binding.
package transport

import (
	"encoding/binary"
	"fmt"
	"io"
)

// CertBundle holds the in-memory PEM blobs the supervisor minted and
// passed to us via stdin.
type CertBundle struct {
	CAPEM         []byte
	ServerCertPEM []byte
	ServerKeyPEM  []byte
}

// Max payload size per part — prevents DoS via crafted header.
const maxPartBytes = 1 << 20 // 1 MiB

// ReadCertBlob parses 3 length-prefixed PEM payloads (CA, server cert,
// server key) from r. Matches kubilitics-backend's
// internal/ai/certs/mint.go:WriteStdinBlob framing.
func ReadCertBlob(r io.Reader) (*CertBundle, error) {
	parts := make([][]byte, 3)
	for i := range parts {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, fmt.Errorf("read length: %w", err)
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n > maxPartBytes {
			return nil, fmt.Errorf("payload too large: %d bytes", n)
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("read payload %d: %w", i, err)
		}
		parts[i] = buf
	}
	return &CertBundle{CAPEM: parts[0], ServerCertPEM: parts[1], ServerKeyPEM: parts[2]}, nil
}
