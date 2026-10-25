//go:build windows

package ipc

import (
	"errors"
	"net"
)

var errWindows = errors.New("the control socket is not supported on Windows yet")

func prepareDir(string) error { return errWindows }

func listenUnix(string) (net.Listener, error) { return nil, errWindows }
