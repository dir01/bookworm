package zim

import "bytes"

// Mimetype describes one of the three possible
// fixed Mimetypes for a Directory Entry.
type Mimetype uint16

// Possible fixed Mimetype values for Directory Entry.
const (
	MimetypeDeletedEntry  = Mimetype(0xFFFD)
	MimetypeLinkTarget    = Mimetype(0xFFFE)
	MimetypeRedirectEntry = Mimetype(0xFFFF)
)

func (z *File) readMimetypeList() {
	seek(z.f, int64(z.header.mimeListPos))
	for {
		var mimetype = readNullTerminatedSlice(z.f)
		if len(mimetype) == 0 {
			break
		}
		z.mimetypeList = append(z.mimetypeList, string(bytes.ToLower(bytes.TrimSpace(mimetype))))
	}
}

// MimetypeList returns the internal Mimetype list of the ZIM file.
func (z *File) MimetypeList() []string {
	return z.mimetypeList
}
