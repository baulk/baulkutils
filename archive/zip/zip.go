package zip

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/baulk/bkz/archive/basics"
	"golang.org/x/text/encoding"
)

// CompressionMethod compress method see https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT
type CompressionMethod uint16

// CompressionMethod
// value
const (
	Store   CompressionMethod = 0
	Deflate CompressionMethod = 8
	BZIP2   CompressionMethod = 12
	LZMA    CompressionMethod = 14
	LZMA2   CompressionMethod = 33
	ZSTD    CompressionMethod = 93
	XZ      CompressionMethod = 95
	JPEG    CompressionMethod = 96
	WavPack CompressionMethod = 97
	PPMd    CompressionMethod = 98
	AES     CompressionMethod = 99
)

func init() {
	zipRegisterDecompressor()
	zipRegisterCompressor()
}

// Matched magic
func Matched(buf []byte) bool {
	return (len(buf) > 3 && buf[0] == 0x50 && buf[1] == 0x4B &&
		(buf[2] == 0x3 || buf[2] == 0x5 || buf[2] == 0x7) &&
		(buf[3] == 0x4 || buf[3] == 0x6 || buf[3] == 0x8))
}

// Extractor todo
type Extractor struct {
	fd  *os.File
	zr  *zip.Reader
	dec *encoding.Decoder
	es  *basics.ExtractSetting
}

// NewExtractor new extractor
func NewExtractor(fd *os.File, es *basics.ExtractSetting) (*Extractor, error) {
	st, err := fd.Stat()
	if err != nil {
		fd.Close()
		return nil, err
	}
	zr, err := zip.NewReader(fd, st.Size())
	if err != nil {
		fd.Close()
		return nil, err
	}
	e := &Extractor{fd: fd, zr: zr, es: es}
	if ens := os.Getenv("ZIP_ENCODING"); len(ens) != 0 {
		e.initializeEncoder(ens)
	}
	return e, nil
}

// Close fd
func (e *Extractor) Close() error {
	return e.fd.Close()
}

func (e *Extractor) extractSymlink(p, destination string, zf *zip.File) error {
	r, err := zf.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	lnk, err := ioutil.ReadAll(io.LimitReader(r, 32678))
	if err != nil {
		return err
	}
	lnkp := strings.TrimSpace(string(lnk))
	if filepath.IsAbs(lnkp) {
		return basics.SymbolicLink(filepath.Clean(lnkp), p)
	}
	oldname := filepath.Join(filepath.Dir(p), lnkp)
	return basics.SymbolicLink(oldname, p)
}

func (e *Extractor) extractFile(p, destination string, zf *zip.File) error {
	r, err := zf.Open()
	if err != nil {
		if !e.es.IgnoreError {
			return err
		}
	}
	defer r.Close()
	return basics.WriteDisk(r, p, zf.FileHeader.Mode())
}

// Extract file
func (e *Extractor) Extract(destination string) error {
	for _, file := range e.zr.File {
		out := filepath.Join(destination, file.Name)
		if !basics.IsRelativePath(destination, out) {
			if e.es.IgnoreError {
				continue
			}
			return basics.ErrRelativePathEscape
		}
		fi := file.FileInfo()
		if fi.IsDir() {
			if err := os.MkdirAll(out, fi.Mode()); err != nil {
				if !e.es.IgnoreError {
					return err
				}
			}
			continue
		}
		if e.es.OnEntry != nil {
			if err := e.es.OnEntry(file.Name, fi); err != nil {
				return err
			}
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			if err := e.extractSymlink(out, destination, file); err != nil {
				if !e.es.IgnoreError {
					return err
				}
			}
			continue
		}
		if err := e.extractFile(out, destination, file); err != nil {
			if !e.es.IgnoreError {
				return err
			}
		}
	}
	return nil
}
