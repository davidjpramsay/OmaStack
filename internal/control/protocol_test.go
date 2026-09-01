package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRequestValidation(t *testing.T) {
	valid := Request{Version: ProtocolVersion, ID: "abc", Method: "service.start", Params: json.RawMessage(`{"target":"x"}`)}
	if err := ValidateRequest(valid); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"", "Start", "a/b", strings.Repeat("a", 65)} {
		request := valid
		request.Method = method
		if ValidateRequest(request) == nil {
			t.Errorf("accepted method %q", method)
		}
	}
}

func TestEncodeAndResponseConstructors(t *testing.T) {
	var encoded bytes.Buffer
	response := Success("request", map[string]string{"status": "ok"})
	if err := Encode(&encoded, response); err != nil {
		t.Fatal(err)
	}
	var decoded Response
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Version != ProtocolVersion || !decoded.OK || decoded.ID != "request" {
		t.Fatalf("response = %#v", decoded)
	}
	long := strings.Repeat("x", 600)
	failure := Failure("request", "failed", long)
	if failure.Error == nil || len(failure.Error.Message) != 512 || !strings.HasSuffix(failure.Error.Message, "…") {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestClientCallSuccessAndDaemonError(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "control.sock")
	listener, err := net.Listen("unix", socket)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox does not permit Unix sockets")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		for count := 0; count < 2; count++ {
			connection, err := listener.Accept()
			if err != nil {
				done <- err
				return
			}
			if err = SameUser(connection); err != nil {
				connection.Close()
				done <- err
				return
			}
			request, err := Decode(connection)
			if err == nil {
				if request.Method == "test.ok" {
					err = Encode(connection, Success(request.ID, map[string]string{"value": "ready"}))
				} else {
					err = Encode(connection, Failure(request.ID, "denied", "not allowed"))
				}
			}
			connection.Close()
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	client := Client{Socket: socket, Timeout: time.Second}
	var result map[string]string
	if err := client.Call(context.Background(), "test.ok", map[string]string{"input": "x"}, &result); err != nil || result["value"] != "ready" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if err := client.Call(context.Background(), "test.fail", nil, nil); err == nil || !strings.Contains(err.Error(), "denied: not allowed") {
		t.Fatalf("daemon error = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := client.Call(context.Background(), "INVALID", nil, nil); err == nil {
		t.Fatal("invalid method accepted")
	}
}

func TestSameUserRejectsNonUnixConnection(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	if err := SameUser(server); err == nil || !strings.Contains(err.Error(), "not Unix-domain") {
		t.Fatalf("non-Unix connection error = %v", err)
	}
}

func TestDecodeRejectsUnknownAndOversizedRequests(t *testing.T) {
	if _, err := Decode(bytes.NewBufferString(`{"version":1,"id":"x","method":"ping","unknown":true}` + "\n")); err == nil {
		t.Fatal("unknown field accepted")
	}
	large := strings.Repeat("x", MaxMessageBytes+1)
	if _, err := Decode(strings.NewReader(large)); err == nil {
		t.Fatal("oversized request accepted")
	}
}
