package image

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

type Format string

const (
	FormatISO     Format = "ISO"
	FormatCDI     Format = "CDI"
	FormatCUE     Format = "CUE"
	FormatBIN     Format = "BIN"
	FormatNRG     Format = "NRG"
	FormatXISO    Format = "XISO"
	FormatMDS     Format = "MDS"
	FormatCCD     Format = "CCD"
	FormatUnknown Format = "Unknown"
)

type ImageInfo struct {
	Format     Format
	Path       string
	Size       int64
	Title      string
	Tracks     []TrackInfo
	Sessions   int
	Platform   string
	RawDetails map[string]string
}

type TrackInfo struct {
	Number    int
	Type      string
	Sectors   int64
	StartLBA  int64
	SizeBytes int64
}

func DetectFormat(path string) (Format, error) {
	ext := strings.ToUpper(path)
	switch {
	case strings.HasSuffix(ext, ".CDI"):
		return FormatCDI, nil
	case strings.HasSuffix(ext, ".CUE"):
		return FormatCUE, nil
	case strings.HasSuffix(ext, ".NRG"):
		return FormatNRG, nil
	case strings.HasSuffix(ext, ".MDS"):
		return FormatMDS, nil
	case strings.HasSuffix(ext, ".CCD"):
		return FormatCCD, nil
	case strings.HasSuffix(ext, ".XISO") || strings.HasSuffix(ext, ".ISO"):
		f, err := os.Open(path)
		if err != nil {
			return FormatUnknown, err
		}
		defer f.Close()
		return detectISOorXISO(f)
	case strings.HasSuffix(ext, ".BIN"):
		return FormatBIN, nil
	case strings.HasSuffix(ext, ".IMG"):
		return FormatBIN, nil
	default:
		return FormatUnknown, fmt.Errorf("unrecognized file extension: %s", path)
	}
}

func detectISOorXISO(f *os.File) (Format, error) {
	buf := make([]byte, 32)

	if _, err := f.Seek(0, io.SeekStart); err == nil {
		if n, _ := f.Read(buf); n >= 16 {
			if string(buf[:15]) == "MICROSOFT*XBOX" {
				return FormatXISO, nil
			}
		}
	}

	if _, err := f.Seek(0x8001, io.SeekStart); err == nil {
		if n, _ := f.Read(buf); n >= 5 {
			if string(buf[:5]) == "CD001" {
				return FormatISO, nil
			}
		}
	}

	info, err := f.Stat()
	if err == nil && info.Size() > 16 {
		if _, err := f.Seek(info.Size()-8, io.SeekStart); err == nil {
			if n, _ := f.Read(buf); n >= 4 {
				if string(buf[:3]) == "CDI" {
					return FormatCDI, nil
				}
			}
		}
	}

	return FormatISO, nil
}

func Analyze(path string) (*ImageInfo, error) {
	format, err := DetectFormat(path)
	if err != nil {
		return nil, fmt.Errorf("detect format: %w", err)
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	info := &ImageInfo{
		Format: format,
		Path:   path,
		Size:   stat.Size(),
	}

	switch format {
	case FormatCDI:
		return analyzeCDI(info)
	case FormatISO:
		return analyzeISO(info)
	case FormatCUE:
		return analyzeCUE(info)
	case FormatXISO:
		return analyzeXISO(info)
	case FormatNRG:
		return analyzeNRG(info)
	default:
		return info, nil
	}
}

func analyzeISO(info *ImageInfo) (*ImageInfo, error) {
	f, err := os.Open(info.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, 2048)
	if _, err := f.Seek(0x8000, io.SeekStart); err != nil {
		return info, nil
	}
	if _, err := f.Read(buf); err != nil {
		return info, nil
	}

	volumeID := strings.TrimRight(string(buf[40:72]), "\x00 ")
	info.Title = volumeID
	info.Platform = "General"
	info.Sessions = 1
	info.Tracks = []TrackInfo{{
		Number:    1,
		Type:      "Mode1",
		Sectors:   info.Size / 2048,
		SizeBytes: info.Size,
	}}

	if volSetSize := binary.LittleEndian.Uint16(buf[120:122]); volSetSize > 1 {
		info.RawDetails = map[string]string{
			"Volume Set Size": fmt.Sprintf("%d", volSetSize),
		}
	}

	return info, nil
}
