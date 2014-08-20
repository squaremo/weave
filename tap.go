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

// [1] can't get tap output to work, so use pcap instead.
// Unfortunately that also means, at least in this quick-hack version,
// we need to hardcode the interface for pcap, since we cannot inject
// on a tap interface with pcap.

type TapIO struct {
	handle *os.File
	buf    []byte
	po     PacketSink // [1]
}

func NewTapIO(ifName string, bufSz int) (PacketSourceSink, error) {
	handle, err := newTap(ifName)
	if err != nil {
		return nil, err
	}
	po, err := NewPcapO("ethwe") // [1]
	return &TapIO{handle: handle, buf: make([]byte, bufSz), po: po}, nil
}

func NewTapO(ifName string) (po PacketSink, err error) {
	// [1]
	//
	// handle, err := newTap(ifName)
	// if err != nil {
	// 	return nil, err
	// }
	// return &TapIO{handle: handle}, nil
	po, err = NewPcapO("ethwe")
	return
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
	// [1]
	//
	// _, err := po.handle.Write(data)
	// return err
	return po.po.WritePacket(data)
}
