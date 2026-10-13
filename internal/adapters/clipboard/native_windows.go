//go:build windows

package clipboard

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// win32Clipboard uses the Win32 clipboard API (CF_UNICODETEXT) directly,
// without cgo.
type win32Clipboard struct{}

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procOpen         = user32.NewProc("OpenClipboard")
	procClose        = user32.NewProc("CloseClipboard")
	procEmpty        = user32.NewProc("EmptyClipboard")
	procGetData      = user32.NewProc("GetClipboardData")
	procSetData      = user32.NewProc("SetClipboardData")
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

func detectNative() native {
	if user32.Load() != nil || kernel32.Load() != nil {
		return nil
	}
	return win32Clipboard{}
}

// open retries briefly: another process may hold the clipboard.
func open() error {
	var err error
	for range 10 {
		var r uintptr
		if r, _, err = procOpen.Call(0); r != 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("open clipboard: %w", err)
}

func (win32Clipboard) copy(text string) error {
	u16, err := windows.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := open(); err != nil {
		return err
	}
	defer procClose.Call()
	procEmpty.Call()

	size := uintptr(len(u16)) * unsafe.Sizeof(u16[0])
	h, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return fmt.Errorf("copy: GlobalAlloc: %w", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("copy: GlobalLock: %w", err)
	}
	// p points to OS-allocated (non-Go) memory, locked until GlobalUnlock.
	copy(unsafe.Slice((*uint16)(unsafe.Add(nil, p)), len(u16)), u16)
	procGlobalUnlock.Call(h)
	if r, _, err := procSetData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("copy: SetClipboardData: %w", err)
	}
	return nil // the clipboard owns h now
}

func (win32Clipboard) paste() (string, error) {
	if err := open(); err != nil {
		return "", err
	}
	defer procClose.Call()
	h, _, err := procGetData.Call(cfUnicodeText)
	if h == 0 {
		return "", fmt.Errorf("paste: GetClipboardData: %w", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return "", fmt.Errorf("paste: GlobalLock: %w", err)
	}
	defer procGlobalUnlock.Call(h)
	return windows.UTF16PtrToString((*uint16)(unsafe.Add(nil, p))), nil
}
