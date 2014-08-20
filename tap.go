package weave

/*
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <linux/if.h>
#include <linux/if_tun.h>

#define IFREQ_SIZE sizeof(struct ifreq)
*/
import "C"

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	flagTruncated = C.TUN_PKT_STRIP
)

type ifReq struct {
	Name  [C.IFNAMSIZ]byte
	Flags uint16
	pad   [C.IFREQ_SIZE - C.IFNAMSIZ - 2]byte
}

type TapIO struct {
	handle *os.File
	buf    []byte
}

func NewTapIO(ifName string, bufSz int) (PacketSourceSink, error) {
	handle, err := newTap(ifName)
	if err != nil {
		return nil, err
	}
	return &TapIO{handle: handle, buf: make([]byte, bufSz)}, nil
}

func NewTapO(ifName string) (PacketSink, error) {
	handle, err := newTap(ifName)
	if err != nil {
		return nil, err
	}
	return &TapIO{handle: handle}, nil
}

func newTap(ifName string) (*os.File, error) {
	file, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var req ifReq
	req.Flags = C.IFF_TAP | C.IFF_ONE_QUEUE | C.IFF_NO_PI
	copy(req.Name[:C.IFNAMSIZ], ifName)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TUNSETIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return nil, errno
	}
	return file, nil
}

func (pi *TapIO) ReadPacket() ([]byte, error) {
	n, err := pi.handle.Read(pi.buf)
	if err != nil {
		return nil, err
	}
	return pi.buf[:n], nil
}

func (po *TapIO) WritePacket(data []byte) error {
	_, err := po.handle.Write(data)
	return err
}
