package zim

import (
	"encoding/binary"
	"fmt"
)

const (
	zimMagic       = 0x44D495A
	headerSize     = 80
	noMainPage     = 0xFFFFFFFF
	noTitlePtrList = 0xFFFFFFFFFFFFFFFF
)

type header struct {
	Magic         uint32
	MajorVersion  uint16
	MinorVersion  uint16
	UUID          [16]byte
	EntryCount    uint32
	ClusterCount  uint32
	URLPtrPos     uint64
	TitlePtrPos   uint64
	ClusterPtrPos uint64
	MIMEListPos   uint64
	MainPage      uint32
	LayoutPage    uint32
	ChecksumPos   uint64
}

func parseHeader(data []byte) (header, error) {
	if len(data) < headerSize {
		return header{}, fmt.Errorf("%w: header too short (%d bytes)", ErrInvalidMagic, len(data))
	}

	var h header
	le := binary.LittleEndian

	h.Magic = le.Uint32(data[0:4])
	if h.Magic != zimMagic {
		return header{}, fmt.Errorf("%w: got 0x%X, want 0x%X", ErrInvalidMagic, h.Magic, zimMagic)
	}

	h.MajorVersion = le.Uint16(data[4:6])
	h.MinorVersion = le.Uint16(data[6:8])

	if h.MajorVersion != 5 && h.MajorVersion != 6 {
		return header{}, fmt.Errorf("%w: version %d", ErrUnsupportedVersion, h.MajorVersion)
	}

	copy(h.UUID[:], data[8:24])

	h.EntryCount = le.Uint32(data[24:28])
	h.ClusterCount = le.Uint32(data[28:32])
	h.URLPtrPos = le.Uint64(data[32:40])
	h.TitlePtrPos = le.Uint64(data[40:48])
	h.ClusterPtrPos = le.Uint64(data[48:56])
	h.MIMEListPos = le.Uint64(data[56:64])
	h.MainPage = le.Uint32(data[64:68])
	h.LayoutPage = le.Uint32(data[68:72])
	h.ChecksumPos = le.Uint64(data[72:80])

	return h, nil
}
