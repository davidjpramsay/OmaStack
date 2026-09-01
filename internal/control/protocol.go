package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"time"
)

const (
	ProtocolVersion  = 1
	MaxMessageBytes  = 1 << 20
	MaxResponseBytes = 8 << 20
)

var methodPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,63}$`)

type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TargetParams struct {
	Target string `json:"target"`
}
type LogsParams struct {
	Target string `json:"target"`
	Lines  int    `json:"lines"`
	Query  string `json:"query,omitempty"`
}

func ValidateRequest(request Request) error {
	if request.Version != ProtocolVersion {
		return errors.New("unsupported protocol version")
	}
	if len(request.ID) < 1 || len(request.ID) > 64 {
		return errors.New("invalid request id")
	}
	if !methodPattern.MatchString(request.Method) {
		return errors.New("invalid method")
	}
	if len(request.Params) > MaxMessageBytes/2 {
		return errors.New("params too large")
	}
	return nil
}

func Decode(reader io.Reader) (Request, error) {
	limited := bufio.NewReader(io.LimitReader(reader, MaxMessageBytes+1))
	line, err := limited.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return Request{}, err
	}
	if len(line) > MaxMessageBytes {
		return Request{}, errors.New("request too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("invalid request: %w", err)
	}
	if err := ValidateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func Encode(writer io.Writer, response Response) error {
	response.Version = ProtocolVersion
	return json.NewEncoder(writer).Encode(response)
}

type Client struct {
	Socket  string
	Timeout time.Duration
}

func (c Client) Call(ctx context.Context, method string, params any, result any) error {
	if !methodPattern.MatchString(method) {
		return errors.New("invalid method")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(callCtx, "unix", c.Socket)
	if err != nil {
		return err
	}
	defer connection.Close()
	if deadline, ok := callCtx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	request := Request{Version: ProtocolVersion, ID: requestID(), Method: method, Params: raw}
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return err
	}
	var response Response
	decoder := json.NewDecoder(io.LimitReader(connection, MaxResponseBytes))
	if err := decoder.Decode(&response); err != nil {
		return err
	}
	if response.Version != ProtocolVersion || response.ID != request.ID {
		return errors.New("invalid daemon response")
	}
	if !response.OK {
		if response.Error == nil {
			return errors.New("daemon request failed")
		}
		return fmt.Errorf("%s: %s", response.Error.Code, response.Error.Message)
	}
	if result == nil {
		return nil
	}
	encoded, err := json.Marshal(response.Result)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, result)
}

func requestID() string { return fmt.Sprintf("%x", time.Now().UnixNano()) }

func Failure(id, code, message string) Response {
	if len(message) > 512 {
		message = message[:509] + "…"
	}
	return Response{ID: id, OK: false, Error: &Error{Code: code, Message: message}}
}

func Success(id string, result any) Response { return Response{ID: id, OK: true, Result: result} }
