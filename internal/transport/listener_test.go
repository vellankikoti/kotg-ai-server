package transport

import (
	"bytes"
	"strings"
	"testing"
)

func TestBindLocalhostReturnsPort(t *testing.T) {
	lis, port, err := BindLocalhost()
	if err != nil {
		t.Fatalf("BindLocalhost: %v", err)
	}
	defer lis.Close()
	if port <= 0 {
		t.Errorf("port = %d, want >0", port)
	}
}

func TestWriteReadyFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReady(&buf, 12345); err != nil {
		t.Fatalf("WriteReady: %v", err)
	}
	s := buf.String()
	if !strings.HasPrefix(s, "READY 12345") || !strings.HasSuffix(s, "\n") {
		t.Errorf("WriteReady format wrong: %q", s)
	}
}
