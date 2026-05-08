package image

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// NRG (Nero Burning ROM) format parser.
//
// NRG v1: footer at end of file with "NERO" magic
// NRG v2: footer with "NER5" magic
// Both use chunk-based structure with "CUEX", "DAOX", "CDTX", "ETN2", "SINF", "MTYP" chunks

const (
	nrgV1Magic = "NERO"
	nrgV2Magic = "NER5"
)

type NRGChunk struct {
	ID   string
	Data []byte
}

func analyzeNRG(info *ImageInfo) (*ImageInfo, error) {
	f, err := os.Open(info.Path)
	if err != nil {
		return nil, fmt.Errorf("open NRG: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// NRG footer: last 12 bytes for v2, last 8 bytes for v1
	footerSize := int64(12)
	if stat.Size() < footerSize {
		return nil, fmt.Errorf("file too small for NRG")
	}

	footerBuf := make([]byte, footerSize)
	if _, err := f.Seek(stat.Size()-footerSize, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(f, footerBuf); err != nil {
		return nil, err
	}

	var version string
	var firstChunkOffset int64

	magic := string(footerBuf[4:8])
	switch magic {
	case nrgV1Magic:
		version = "1"
		firstChunkOffset = int64(binary.LittleEndian.Uint32(footerBuf[0:4]))
	case nrgV2Magic:
		version = "2"
		firstChunkOffset = int64(binary.BigEndian.Uint64(footerBuf[0:8]))
	default:
		return nil, fmt.Errorf("not a valid NRG file (magic: %q)", magic)
	}

	// Read chunks
	var tracks []TrackInfo
	var mediaType string

	if _, err := f.Seek(firstChunkOffset, io.SeekStart); err != nil {
		return nil, err
	}

	for {
		chunkHeader := make([]byte, 8)
		if _, err := io.ReadFull(f, chunkHeader); err != nil {
			break
		}

		chunkID := string(chunkHeader[:4])
		chunkSize := int64(binary.BigEndian.Uint32(chunkHeader[4:8]))

		chunkData := make([]byte, chunkSize)
		if chunkSize > 0 {
			if _, err := io.ReadFull(f, chunkData); err != nil {
				break
			}
		}

		switch chunkID {
		case "CUEX": // Cue points (NRG v2)
			tracks = parseNRGCuex(chunkData)
		case "CUES": // Cue points (NRG v1)
			tracks = parseNRGCues(chunkData)
		case "MTYP":
			if len(chunkData) >= 4 {
				mediaType = fmt.Sprintf("0x%08X", binary.BigEndian.Uint32(chunkData[:4]))
			}
		case "SINF": // Session info
			// Contains number of tracks in session
		case "DAOX", "DAOI": // Disc-at-once info
			// Contains detailed track layout
		case "END!":
			break
		}
	}

	info.Sessions = 1
	info.Tracks = tracks
	info.Platform = "General"

	if len(tracks) == 0 {
		info.Tracks = []TrackInfo{{
			Number:    1,
			Type:      "Mode1",
			Sectors:   info.Size / 2048,
			SizeBytes: info.Size,
		}}
	}

	if info.RawDetails == nil {
		info.RawDetails = make(map[string]string)
	}
	info.RawDetails["NRG Version"] = version
	if mediaType != "" {
		info.RawDetails["Media Type"] = mediaType
	}

	return info, nil
}

func parseNRGCuex(data []byte) []TrackInfo {
	// CUEX entries: 8 bytes each (control/adr, track, index, MSF/LBA)
	var tracks []TrackInfo
	entryCount := len(data) / 8

	for i := 0; i < entryCount; i++ {
		entry := data[i*8 : (i+1)*8]
		control := entry[0]
		trackNum := int(entry[1])
		_ = control

		if trackNum == 0xAA || trackNum == 0 {
			continue // lead-in/lead-out
		}

		lba := int64(binary.BigEndian.Uint32(entry[4:8]))

		tracks = append(tracks, TrackInfo{
			Number:   trackNum,
			StartLBA: lba,
			Type:     "Mode1",
		})
	}

	// Calculate sector counts from LBA differences
	for i := range tracks {
		if i+1 < len(tracks) {
			tracks[i].Sectors = tracks[i+1].StartLBA - tracks[i].StartLBA
		}
	}

	return tracks
}

func parseNRGCues(data []byte) []TrackInfo {
	// CUES: 6-byte entries (control, track, index, 3-byte MSF)
	var tracks []TrackInfo
	entryCount := len(data) / 6

	for i := 0; i < entryCount; i++ {
		entry := data[i*6 : (i+1)*6]
		trackNum := int(entry[1])

		if trackNum == 0xAA || trackNum == 0 {
			continue
		}

		lba := int64(entry[3])*75*60 + int64(entry[4])*75 + int64(entry[5])

		tracks = append(tracks, TrackInfo{
			Number:   trackNum,
			StartLBA: lba,
			Type:     "Mode1",
		})
	}

	return tracks
}
