package image

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// CDI (DiscJuggler) format parser for Dreamcast disc images.
//
// CDI file layout (simplified):
//   - Track data (raw sectors)
//   - Session descriptors (at end of file, pointed to by footer)
//   - Footer (last 8 bytes: "CDI" magic + offset to session info)
//
// Each session contains track descriptors with:
//   - Track number, start LBA, total length
//   - Mode (Audio, Mode1, Mode2/Form1, Mode2/Form2)
//   - Session boundaries (critical for Dreamcast boot)

const cdiMagic = "CDI"

type CDIHeader struct {
	Version        uint16
	Sessions       []CDISession
	TotalSectors   int64
	MediaType      string
}

type CDISession struct {
	Number  int
	Tracks  []CDITrack
	StartLBA int64
	EndLBA   int64
}

type CDITrack struct {
	Number    int
	StartLBA  int64
	TotalLBA  int64
	Mode      string
	SectorSize int
	IsAudio   bool
}

func analyzeCDI(info *ImageInfo) (*ImageInfo, error) {
	f, err := os.Open(info.Path)
	if err != nil {
		return nil, fmt.Errorf("open CDI: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// CDI footer: last 8 bytes contain offset pointer
	if stat.Size() < 16 {
		return nil, fmt.Errorf("file too small to be a valid CDI")
	}

	footerBuf := make([]byte, 16)
	if _, err := f.Seek(stat.Size()-16, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to footer: %w", err)
	}
	if _, err := io.ReadFull(f, footerBuf); err != nil {
		return nil, fmt.Errorf("read footer: %w", err)
	}

	magic := string(footerBuf[:3])
	if magic != cdiMagic {
		return nil, fmt.Errorf("not a valid CDI file (magic: %q)", magic)
	}

	version := binary.LittleEndian.Uint16(footerBuf[3:5])
	headerOffset := int64(binary.LittleEndian.Uint32(footerBuf[8:12]))

	header := CDIHeader{
		Version: version,
	}

	// Read number of sessions
	if _, err := f.Seek(headerOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to header: %w", err)
	}

	sessionCountBuf := make([]byte, 2)
	if _, err := io.ReadFull(f, sessionCountBuf); err != nil {
		return nil, fmt.Errorf("read session count: %w", err)
	}
	sessionCount := int(binary.LittleEndian.Uint16(sessionCountBuf))

	// Read track position table (array of uint16 offsets)
	trackPosTable := make([]byte, sessionCount*2)
	if _, err := io.ReadFull(f, trackPosTable); err != nil {
		return nil, fmt.Errorf("read track position table: %w", err)
	}

	var allTracks []TrackInfo
	var totalSectors int64

	for i := 0; i < sessionCount; i++ {
		session := CDISession{Number: i + 1}

		// Each session: seek to its position and read track info
		// The format has variable-length records, so we parse sequentially
		trackCountBuf := make([]byte, 2)
		if _, err := io.ReadFull(f, trackCountBuf); err != nil {
			break
		}
		trackCount := int(binary.LittleEndian.Uint16(trackCountBuf))

		for t := 0; t < trackCount; t++ {
			track := parseCDITrack(f)
			session.Tracks = append(session.Tracks, track)

			trackType := "Mode1"
			if track.IsAudio {
				trackType = "Audio"
			} else if track.Mode != "" {
				trackType = track.Mode
			}

			allTracks = append(allTracks, TrackInfo{
				Number:    track.Number,
				Type:      trackType,
				Sectors:   track.TotalLBA,
				StartLBA:  track.StartLBA,
				SizeBytes: track.TotalLBA * int64(track.SectorSize),
			})
			totalSectors += track.TotalLBA
		}

		header.Sessions = append(header.Sessions, session)
	}

	info.Platform = detectCDIPlatform(sessionCount, allTracks)
	info.Sessions = sessionCount
	info.Tracks = allTracks
	info.Title = extractCDITitle(f, headerOffset)

	if info.RawDetails == nil {
		info.RawDetails = make(map[string]string)
	}
	info.RawDetails["CDI Version"] = fmt.Sprintf("%d", version)
	info.RawDetails["Total Sectors"] = fmt.Sprintf("%d", totalSectors)
	info.RawDetails["Media Type"] = header.MediaType

	return info, nil
}

func parseCDITrack(f *os.File) CDITrack {
	track := CDITrack{}

	buf := make([]byte, 4)

	// Track number
	if _, err := io.ReadFull(f, buf[:2]); err != nil {
		return track
	}
	track.Number = int(binary.LittleEndian.Uint16(buf[:2]))

	// Start LBA
	if _, err := io.ReadFull(f, buf); err != nil {
		return track
	}
	track.StartLBA = int64(binary.LittleEndian.Uint32(buf))

	// Total LBA (length in sectors)
	if _, err := io.ReadFull(f, buf); err != nil {
		return track
	}
	track.TotalLBA = int64(binary.LittleEndian.Uint32(buf))

	// Mode
	modeBuf := make([]byte, 1)
	if _, err := io.ReadFull(f, modeBuf); err != nil {
		return track
	}
	modeVal := modeBuf[0]

	switch modeVal {
	case 0:
		track.IsAudio = true
		track.Mode = "Audio"
		track.SectorSize = 2352
	case 1:
		track.Mode = "Mode1"
		track.SectorSize = 2048
	case 2:
		track.Mode = "Mode2/Form1"
		track.SectorSize = 2048
	default:
		track.Mode = fmt.Sprintf("Mode%d", modeVal)
		track.SectorSize = 2352
	}

	return track
}

func detectCDIPlatform(sessions int, tracks []TrackInfo) string {
	// Dreamcast games have 2 sessions: Audio (session 1) + Data (session 2)
	if sessions == 2 {
		hasAudio := false
		hasData := false
		for _, t := range tracks {
			if t.Type == "Audio" {
				hasAudio = true
			}
			if t.Type == "Mode2/Form1" || t.Type == "Mode1" {
				hasData = true
			}
		}
		if hasAudio && hasData {
			return "Sega Dreamcast"
		}
	}
	return "General"
}

func extractCDITitle(f *os.File, headerOffset int64) string {
	// Try to read volume label from the data track's ISO header
	// The data track in session 2 typically has a CD001 volume descriptor
	buf := make([]byte, 2048)

	// Search for ISO volume descriptor in common locations
	searchOffsets := []int64{45000 * 2048, 0, 150 * 2048} // approximate data track offsets
	for _, off := range searchOffsets {
		if _, err := f.Seek(off+0x8000, io.SeekStart); err != nil {
			continue
		}
		if _, err := io.ReadFull(f, buf); err != nil {
			continue
		}
		if string(buf[1:6]) == "CD001" {
			vol := string(buf[40:72])
			for i := range vol {
				if vol[i] == 0 {
					vol = vol[:i]
					break
				}
			}
			if vol != "" {
				return vol
			}
		}
	}
	return ""
}
