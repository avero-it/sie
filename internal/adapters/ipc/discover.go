package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RemoveStale deletes the sockets in dir that nobody listens on any more.
func RemoveStale(dir string) {
	for _, s := range sockets(dir) {
		nc, err := net.DialTimeout("unix", s.path, staleDialLimit)
		if err == nil {
			_ = nc.Close()
			continue
		}
		if !os.IsPermission(err) {
			_ = os.Remove(s.path)
		}
	}
}

type socketFile struct {
	pid  int
	path string
}

func sockets(dir string) []socketFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []socketFile
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, socketSuffix) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSuffix(name, socketSuffix))
		if err != nil {
			continue
		}
		out = append(out, socketFile{pid, filepath.Join(dir, name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pid < out[j].pid })
	return out
}

// Client is a connection to one instance.
type Client struct {
	nc  net.Conn
	r   *bufio.Reader
	mu  sync.Mutex
	id  int
	PID int
}

// Dial connects to the instance with the given socket file.
func Dial(path string, timeout time.Duration) (*Client, error) {
	nc, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{nc: nc, r: bufio.NewReaderSize(nc, 64<<10)}, nil
}

// Close ends the connection.
func (c *Client) Close() error { return c.nc.Close() }

// Call sends a request and decodes the result into out (nil to ignore it).
// A failure of the editor is returned as *Error.
func (c *Client) Call(method string, params, out any, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id++
	req := map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method}
	if params != nil {
		req["params"] = params
	}
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_ = c.nc.SetDeadline(time.Now().Add(timeout))
	if _, err := c.nc.Write(append(line, '\n')); err != nil {
		return err
	}
	data, err := readLine(c.r)
	if err != nil {
		return err
	}
	var resp response
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("bad response: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Result) > 0 {
		return json.Unmarshal(resp.Result, out)
	}
	return nil
}

// Instance is a running sie found in the socket directory.
type Instance struct {
	InfoResult
	Socket string `json:"socket"`
}

// Discover lists the running instances, removing stale sockets, most
// recently used first.
func Discover(dir string, timeout time.Duration) []Instance {
	var out []Instance
	for _, s := range sockets(dir) {
		c, err := Dial(s.path, staleDialLimit)
		if err != nil {
			if !errors.Is(err, os.ErrPermission) {
				_ = os.Remove(s.path)
			}
			continue
		}
		var info InfoResult
		err = c.Call(MethodInfo, nil, &info, timeout)
		_ = c.Close()
		if err != nil {
			continue
		}
		out = append(out, Instance{InfoResult: info, Socket: s.path})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastActivity.After(out[j].LastActivity) })
	return out
}
