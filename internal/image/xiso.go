package image

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// XISO (Xbox ISO) parser.
//
// XISO layout:
//   - Header: "MICROSOFT*XBOX"MEDIA at offset 0, or "XBOX" at sector 0
//   - Root directory table at a specified offset
//   - File entries with name, offset, size attributes
//
// The XDFS (Xbox Disc Filing System) uses a tree structure similar to ISO 9660
// but with a different on-disc layout.

const xisoMagicMS = "MICROSOFT*XBOX"

func analyzeXISO(info *ImageInfo) (*ImageInfo, error) {
	f, err := os.Open(info.Path)
	if err != nil {
		return nil, fmt.Errorf("open XISO: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 2048)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek: %w", err)
	}
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	if string(buf[:15]) != xisoMagicMS {
		return nil, fmt.Errorf("not a valid XISO file")
	}

	info.Platform = "Microsoft Xbox"
	info.Sessions = 1

	// Root directory sector
	rootDirSector := int64(binary.LittleEndian.Uint32(buf[20:24]))
	rootDirSize := int64(binary.LittleEndian.Uint32(buf[24:28]))

	if info.RawDetails == nil {
		info.RawDetails = make(map[string]string)
	}
	info.RawDetails["Root Dir Sector"] = fmt.Sprintf("%d", rootDirSector)
	info.RawDetails["Root Dir Size"] = fmt.Sprintf("%d bytes", rootDirSize)

	// Try to extract a title from the first file entries
	title := extractXISOGameTitle(f, rootDirSector, rootDirSize)
	if title != "" {
		info.Title = title
	} else {
		info.Title = "Xbox Game"
	}

	// Estimate track info
	info.Tracks = []TrackInfo{{
		Number:    1,
		Type:      "Mode1",
		Sectors:   info.Size / 2048,
		SizeBytes: info.Size,
	}}

	return info, nil
}

func extractXISOGameTitle(f *os.File, dirSector, dirSize int64) string {
	buf := make([]byte, 2048)
	if _, err := f.Seek(dirSector*2048, io.SeekStart); err != nil {
		return ""
	}

	bytesLeft := dirSize
	for bytesLeft > 0 {
		readSize := int64(len(buf))
		if bytesLeft < readSize {
			readSize = bytesLeft
		}

		entry := make([]byte, readSize)
		if _, err := io.ReadFull(f, entry); err != nil {
			return ""
		}
		bytesLeft -= readSize

		// Parse directory entries
		offset := int64(0)
		for offset+14 < int64(len(entry)) {
			leftLen := int64(binary.LittleEndian.Uint16(entry[offset : offset+2]))
			rightLen := int64(binary.LittleEndian.Uint16(entry[offset+2 : offset+4]))
			nameLen := int(entry[offset+8])

			if nameLen <= 0 || nameLen > 256 || offset+14+int64(nameLen) > int64(len(entry)) {
				break
			}

			name := string(entry[offset+14 : offset+14+int64(nameLen)])

			// Look for default.xbe or similar game executable
			if name == "default.xbe" {
				return "Xbox Game"
			}

			if leftLen == 0 && rightLen == 0 {
				break
			}

			// Skip to next entry (aligned to 4 bytes)
			entryLen := int64(14) + int64(nameLen)
			entryLen = (entryLen + 3) & ^int64(3)
			offset += entryLen
		}
	}

	return ""
}
