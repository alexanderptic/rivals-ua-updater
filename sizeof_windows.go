//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var unsafeSizeofProcessEntry32 = unsafe.Sizeof(windows.ProcessEntry32{})
