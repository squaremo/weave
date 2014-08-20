package weave

import (
	"code.google.com/p/gopacket/pfring"
)

// [1] can't get pfring output to work, so use pcap instead

type PfringIO struct {
	handle *pfring.Ring
	buf    []byte
	po     PacketSink // [1]
}

func NewPfringIO(ifName string, bufSz int) (pio PacketSourceSink, err error) {
	pio, err = newPfringIO(ifName, true, 65535, bufSz)
	return
}

func NewPfringO(ifName string) (po PacketSink, err error) {
	// [1] po, err = newPfringIO(ifName, false, 0, 0)
	po, err = NewPcapO(ifName)
	return
}

func newPfringIO(ifName string, promisc bool, snaplen int, bufSz int) (handle *PfringIO, err error) {
	var flags pfring.Flag
	if promisc {
		flags = pfring.FlagPromisc
	}
	ring, err := pfring.NewRing(ifName, uint32(snaplen), flags)
	if err != nil {
		return
	}
	if err = ring.SetDirection(pfring.ReceiveOnly); err != nil {
		return
	}
	if err = ring.Enable(); err != nil {
		return
	}
	po, err := NewPcapO(ifName) // [1]
	return &PfringIO{handle: ring, buf: make([]byte, bufSz), po: po}, nil
}


func (pi *PfringIO) ReadPacket() ([]byte, error) {
	ci, err := pi.handle.ReadPacketDataTo(pi.buf)
	if err != nil {
		return nil, err
	}
	return pi.buf[:ci.CaptureLength], nil
}

func (po *PfringIO) WritePacket(data []byte) error {
	// [1] return po.handle.WritePacketData(data)
	return po.po.WritePacket(data)
}
