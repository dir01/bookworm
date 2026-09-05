package zim

import "bytes"

const mimeRedirect = 0xFFFF

// parseMIMEList parses the null-terminated MIME type list from raw bytes.
// The list is a sequence of null-terminated UTF-8 strings, terminated by an
// empty string (a lone null byte).
func parseMIMEList(data []byte) []string {
	var types []string
	for len(data) > 0 {
		idx := bytes.IndexByte(data, 0)
		if idx < 0 {
			break
		}
		if idx == 0 {
			break // empty string terminates the list
		}
		types = append(types, string(data[:idx]))
		data = data[idx+1:]
	}
	return types
}
