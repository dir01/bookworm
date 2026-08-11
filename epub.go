package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// epubContainer models META-INF/container.xml, which points at the OPF package
// document via the first <rootfile full-path="...">.
type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// opfPackage models the parts of the OPF package document we care about: the
// Dublin Core metadata block and the manifest (used to detect a cover image).
type opfPackage struct {
	Metadata opfMetadata `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
}

type opfMetadata struct {
	Titles       []string    `xml:"title"`
	Creators     []opfAuthor `xml:"creator"`
	Languages    []string    `xml:"language"`
	Subjects     []string    `xml:"subject"`
	Dates        []string    `xml:"date"`
	Descriptions []string    `xml:"description"`
	Metas        []opfMeta   `xml:"meta"`
}

type opfAuthor struct {
	// FileAs is the opf:file-as attribute, conventionally "Last, First".
	FileAs string `xml:"file-as,attr"`
	Value  string `xml:",chardata"`
}

type opfMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
}

// readEPUBMetadataFromFile extracts book metadata from an .epub file on disk.
func readEPUBMetadataFromFile(path string) (*BookMetadata, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("error opening epub %q: %w", path, err)
	}
	defer zr.Close()
	return readEPUBMetadata(&zr.Reader)
}

// readEPUBMetadataFromBytes extracts book metadata from an in-memory .epub
// (e.g. a blob read out of a ZIM archive).
func readEPUBMetadataFromBytes(b []byte) (*BookMetadata, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("error reading epub bytes: %w", err)
	}
	return readEPUBMetadata(zr)
}

// readEPUBMetadata reads the OPF package document referenced by container.xml
// and maps its Dublin Core metadata onto a BookMetadata. FileType, FilePath and
// SubFilepath are left for the caller to fill in.
func readEPUBMetadata(zr *zip.Reader) (*BookMetadata, error) {
	opfPath, err := epubOPFPath(zr)
	if err != nil {
		return nil, err
	}

	pkg, err := parseOPF(zr, opfPath)
	if err != nil {
		return nil, err
	}

	m := &BookMetadata{}
	if len(pkg.Metadata.Titles) > 0 {
		m.Title = strings.TrimSpace(pkg.Metadata.Titles[0])
	}
	if len(pkg.Metadata.Creators) > 0 {
		m.AuthorLastName, m.AuthorFirstName = splitAuthor(pkg.Metadata.Creators[0])
	}
	if len(pkg.Metadata.Languages) > 0 {
		m.Language = strings.TrimSpace(pkg.Metadata.Languages[0])
	}
	if len(pkg.Metadata.Descriptions) > 0 {
		m.Annotation = strings.TrimSpace(pkg.Metadata.Descriptions[0])
	}
	if len(pkg.Metadata.Dates) > 0 {
		m.Date = strings.TrimSpace(pkg.Metadata.Dates[0])
	}
	var subjects []string
	for _, s := range pkg.Metadata.Subjects {
		if s = strings.TrimSpace(s); s != "" {
			subjects = append(subjects, s)
		}
	}
	m.Genre = strings.Join(subjects, ", ")
	m.HasCover = epubHasCover(pkg)

	return m, nil
}

func epubOPFPath(zr *zip.Reader) (string, error) {
	f := findZipEntry(zr, "META-INF/container.xml")
	if f == nil {
		return "", fmt.Errorf("epub: missing META-INF/container.xml")
	}
	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("epub: opening container.xml: %w", err)
	}
	defer rc.Close()

	var c epubContainer
	if err := xml.NewDecoder(rc).Decode(&c); err != nil {
		return "", fmt.Errorf("epub: parsing container.xml: %w", err)
	}
	if len(c.Rootfiles) == 0 || c.Rootfiles[0].FullPath == "" {
		return "", fmt.Errorf("epub: no rootfile in container.xml")
	}
	return c.Rootfiles[0].FullPath, nil
}

func parseOPF(zr *zip.Reader, opfPath string) (*opfPackage, error) {
	f := findZipEntry(zr, opfPath)
	if f == nil {
		return nil, fmt.Errorf("epub: opf %q not found", opfPath)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("epub: opening opf %q: %w", opfPath, err)
	}
	defer rc.Close()

	var pkg opfPackage
	if err := xml.NewDecoder(rc).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("epub: parsing opf %q: %w", opfPath, err)
	}
	return &pkg, nil
}

// findZipEntry returns the zip entry with the given name, matching
// case-insensitively as a fallback (zip names are technically case-sensitive,
// but "META-INF" casing varies in the wild).
func findZipEntry(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, name) {
			return f
		}
	}
	return nil
}

// splitAuthor derives (last, first) name parts from a dc:creator. It prefers the
// opf:file-as attribute ("Last, First"); otherwise it falls back to the display
// name, splitting "First Last" on the final space.
func splitAuthor(a opfAuthor) (last, first string) {
	if name := strings.TrimSpace(a.FileAs); name != "" {
		if before, after, found := strings.Cut(name, ","); found {
			return strings.TrimSpace(before), strings.TrimSpace(after)
		}
		return name, ""
	}

	name := strings.TrimSpace(a.Value)
	if name == "" {
		return "", ""
	}
	if before, after, found := strings.Cut(name, ","); found {
		return strings.TrimSpace(before), strings.TrimSpace(after)
	}
	if i := strings.LastIndex(name, " "); i >= 0 {
		return strings.TrimSpace(name[i+1:]), strings.TrimSpace(name[:i])
	}
	return name, ""
}

func epubHasCover(pkg *opfPackage) bool {
	for _, m := range pkg.Metadata.Metas {
		if strings.EqualFold(m.Name, "cover") && m.Content != "" {
			return true
		}
	}
	for _, it := range pkg.Manifest.Items {
		if strings.Contains(it.Properties, "cover-image") {
			return true
		}
	}
	return false
}
