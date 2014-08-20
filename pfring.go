package weave

import (
	"code.google.com/p/gopacket/pfring"
)

type PfringIO struct {
	handle *pfring.Ring
	buf    []byte
}

func NewPfringIO(ifName string, bufSz int) (pio PacketSourceSink, err error) {
	pio, err = newPfringIO(ifName, true, 65535, bufSz)
	return
}

func NewPfringO(ifName string) (po PacketSink, err error) {
	po, err = newPfringIO(ifName, false, 0, 0)
	return
}

func newPfringIO(ifName string, promisc bool, snaplen int, bufSz int) (handle *PfringIO, err error) {
	var flags pfring.Flag
	if promisc {
		flags = pfring.FlagPromisc
	}
	ring, err := pfring.NewRing(ifName, uint32(bufSz), flags)
	if err != nil {
		return
	}
	if err = ring.SetDirection(pfring.ReceiveOnly); err != nil {
		return
	}
	if err = ring.Enable(); err != nil {
		return
	}
	return &PfringIO{handle: ring, buf: make([]byte, bufSz)}, nil
}


func (pi *PfringIO) ReadPacket() ([]byte, error) {
	ci, err := pi.handle.ReadPacketDataTo(pi.buf)
	if err != nil {
		return nil, err
	}
	return pi.buf[:ci.CaptureLength], nil
}

func (po *PfringIO) WritePacket(data []byte) error {
	return po.handle.WritePacketData(data)
}
