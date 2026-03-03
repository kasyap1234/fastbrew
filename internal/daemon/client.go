package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"fastbrew/internal/brew"
	"fastbrew/internal/services"
)

var (
	ErrVersionMismatch = errors.New("daemon version mismatch")
	ErrUnavailable     = errors.New("daemon unavailable")
)

type Client struct {
	SocketPath    string
	BinaryVersion string
	Timeout       time.Duration
}

func NewClient(socketPath, binaryVersion string) *Client {
	return &Client{
		SocketPath:    socketPath,
		BinaryVersion: binaryVersion,
		Timeout:       2 * time.Second,
	}
}

func (c *Client) Ping() error {
	return c.call(RequestPing, nil, nil)
}

func (c *Client) Status() (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.call(RequestStatus, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Stats() (*StatsResponse, error) {
	var resp StatsResponse
	if err := c.call(RequestStats, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Warmup() error {
	return c.call(RequestWarmup, nil, nil)
}

func (c *Client) Shutdown() error {
	return c.call(RequestShutdown, nil, nil)
}

func (c *Client) Invalidate(event string) error {
	return c.call(RequestInvalidate, InvalidateRequest{Event: event}, nil)
}

func (c *Client) Search(query string) ([]brew.SearchItem, error) {
	var resp SearchResponse
	if err := c.call(RequestSearch, SearchRequest{Query: query}, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) ListInstalled() ([]brew.PackageInfo, error) {
	var resp ListResponse
	if err := c.call(RequestList, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) Outdated() ([]brew.OutdatedPackage, error) {
	var resp OutdatedResponse
	if err := c.call(RequestOutdated, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) Info(packages []string) ([]PackageInfo, error) {
	var resp InfoResponse
	if err := c.call(RequestInfo, InfoRequest{Packages: packages}, &resp); err != nil {
		return nil, err
	}
	return resp.Packages, nil
}

func (c *Client) Deps(packages []string) ([]string, error) {
	var resp DepsResponse
	if err := c.call(RequestDeps, DepsRequest{Packages: packages}, &resp); err != nil {
		return nil, err
	}
	return resp.Dependencies, nil
}

func (c *Client) Leaves() ([]string, error) {
	var resp LeavesResponse
	if err := c.call(RequestLeaves, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) TapInfo(repo string, installedOnly bool) (*brew.TapInfo, error) {
	var resp TapInfoResponse
	if err := c.call(RequestTapInfo, TapInfoRequest{Repo: repo, InstalledOnly: installedOnly}, &resp); err != nil {
		return nil, err
	}
	return resp.Info, nil
}

func (c *Client) ServicesList(scope string) ([]services.Service, error) {
	var resp ServicesListResponse
	if err := c.call(RequestServices, ServicesListRequest{Scope: scope}, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) SubmitJob(operation string, packages []string, options JobSubmitOptions) (string, error) {
	var resp JobSubmitResponse
	if err := c.call(RequestJobSubmit, JobSubmitRequest{
		Operation: operation,
		Packages:  packages,
		Options:   options,
	}, &resp); err != nil {
		return "", err
	}
	return resp.JobID, nil
}

func (c *Client) JobStatus(jobID string) (*JobView, error) {
	var resp JobStatusResponse
	if err := c.call(RequestJobStatus, JobStatusRequest{JobID: jobID}, &resp); err != nil {
		return nil, err
	}
	return &resp.Job, nil
}

func (c *Client) JobStream(jobID string, fromSeq int, blocking bool) (*JobStreamResponse, error) {
	var resp JobStreamResponse
	if err := c.call(RequestJobStream, JobStreamRequest{
		JobID:    jobID,
		FromSeq:  fromSeq,
		Blocking: blocking,
	}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) call(requestType string, payload interface{}, out interface{}) error {
	conn, err := c.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	if err := c.handshake(encoder, decoder); err != nil {
		return err
	}

	if err := encodeRequest(encoder, requestType, payload); err != nil {
		return err
	}

	response, err := decodeResponse(decoder)
	if err != nil {
		return err
	}
	if !response.OK {
		return mapResponseError(response)
	}

	if out == nil || len(response.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Payload, out); err != nil {
		return fmt.Errorf("failed to decode daemon payload: %w", err)
	}
	return nil
}

func (c *Client) handshake(encoder *json.Encoder, decoder *json.Decoder) error {
	if err := encodeRequest(encoder, RequestHandshake, HandshakeRequest{
		APIVersion:    APIVersion,
		BinaryVersion: c.BinaryVersion,
	}); err != nil {
		return err
	}

	response, err := decodeResponse(decoder)
	if err != nil {
		return err
	}
	if !response.OK {
		return mapResponseError(response)
	}

	return nil
}

func (c *Client) dial() (net.Conn, error) {
	if err := validateSocketSecurity(c.SocketPath); err != nil {
		return nil, err
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	conn, err := net.DialTimeout("unix", c.SocketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return conn, nil
}

func encodeRequest(encoder *json.Encoder, requestType string, payload interface{}) error {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		raw = data
	}

	return encoder.Encode(Request{
		Type:    requestType,
		Payload: raw,
	})
}

func decodeResponse(decoder *json.Decoder) (*Response, error) {
	var response Response
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return &response, nil
}

func mapResponseError(response *Response) error {
	switch response.Code {
	case ResponseCodeVer:
		if response.Error == "" {
			return ErrVersionMismatch
		}
		return fmt.Errorf("%w: %s", ErrVersionMismatch, response.Error)
	default:
		if response.Error == "" {
			return fmt.Errorf("daemon request failed (%s)", response.Code)
		}
		return fmt.Errorf("daemon request failed (%s): %s", response.Code, response.Error)
	}
}

func validateSocketSecurity(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: invalid socket path", ErrUnavailable)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("%w: insecure daemon socket mode %o", ErrUnavailable, info.Mode().Perm())
	}

	return nil
}
