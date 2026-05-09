package image

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// CUE sheet parser for CUE/BIN image pairs.
// Supports: AUDIO, MODE1/2048, MODE2/2352, MODE2/2336, etc.

type CUESheet struct {
	FileName    string
	Tracks      []CUETrack
	Title       string
	Performer   string
	Catalog     string
}

type CUETrack struct {
	Number    int
	DataType  string // "AUDIO", "MODE1/2048", "MODE2/2352", etc.
	Index01LBA int64
	Pregap     int64
	Title      string
	Performer  string
	ISRC       string
	File       string
}

func analyzeCUE(info *ImageInfo) (*ImageInfo, error) {
	cue, err := ParseCUE(info.Path)
	if err != nil {
		return nil, fmt.Errorf("parse CUE: %w", err)
	}

	info.Title = cue.Title
	if info.Title == "" {
		info.Title = filepath.Base(info.Path)
	}

	var tracks []TrackInfo
	for i, ct := range cue.Tracks {
		trackType := "Audio"
		sectorSize := 2352
		if strings.HasPrefix(ct.DataType, "MODE1") {
			trackType = "Mode1"
			if strings.HasSuffix(ct.DataType, "/2048") {
				sectorSize = 2048
			}
		} else if strings.HasPrefix(ct.DataType, "MODE2") {
			trackType = "Mode2"
			if strings.HasSuffix(ct.DataType, "/2336") {
				sectorSize = 2336
			}
		}

		tracks = append(tracks, TrackInfo{
			Number:    ct.Number,
			Type:      trackType,
			StartLBA:  ct.Index01LBA,
			Sectors:   0,
			SizeBytes: 0,
		})

		// Set sector size in RawDetails for later size calculation
		_ = sectorSize
		_ = i
	}

	// Calculate track sizes from BIN file + CUE indices
	binPath := FindBINForCUE(info.Path, cue)
	if binPath != "" {
		if stat, err := os.Stat(binPath); err == nil {
			info.Size = stat.Size()
			binSize := stat.Size()

			// Determine sector size from first track's mode
			defaultSectorSize := 2352
			if len(cue.Tracks) > 0 {
				if strings.HasSuffix(cue.Tracks[0].DataType, "/2048") {
					defaultSectorSize = 2048
				} else if strings.HasSuffix(cue.Tracks[0].DataType, "/2336") {
					defaultSectorSize = 2336
				}
			}

			for i := range tracks {
				startByte := tracks[i].StartLBA * int64(defaultSectorSize)
				var endByte int64
				if i+1 < len(tracks) {
					endByte = tracks[i+1].StartLBA * int64(defaultSectorSize)
				} else {
					endByte = binSize
				}
				if endByte > startByte {
					trackBytes := endByte - startByte
					tracks[i].SizeBytes = trackBytes
					tracks[i].Sectors = trackBytes / int64(defaultSectorSize)
				}
			}
		}
	}

	info.Tracks = tracks
	info.Sessions = 1
	info.Platform = detectCUEPlatform(tracks)

	if info.RawDetails == nil {
		info.RawDetails = make(map[string]string)
	}
	if cue.Catalog != "" {
		info.RawDetails["Catalog"] = cue.Catalog
	}
	if cue.Performer != "" {
		info.RawDetails["Performer"] = cue.Performer
	}

	return info, nil
}

func ParseCUE(path string) (*CUESheet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cue := &CUESheet{
		FileName: path,
	}

	var currentTrack *CUETrack

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		upperLine := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upperLine, "TITLE "):
			title := extractQuoted(line[6:])
			if currentTrack != nil {
				currentTrack.Title = title
			} else {
				cue.Title = title
			}

		case strings.HasPrefix(upperLine, "PERFORMER "):
			perf := extractQuoted(line[10:])
			if currentTrack != nil {
				currentTrack.Performer = perf
			} else {
				cue.Performer = perf
			}

		case strings.HasPrefix(upperLine, "CATALOG "):
			cue.Catalog = strings.TrimSpace(line[8:])

		case strings.HasPrefix(upperLine, "FILE "):
			parts := splitCUELine(line)
			if len(parts) >= 3 {
				fileRef := parts[1]
				if currentTrack != nil {
					currentTrack.File = fileRef
				}
			}

		case strings.HasPrefix(upperLine, "TRACK "):
			parts := splitCUELine(line)
			if len(parts) >= 3 {
				num, _ := strconv.Atoi(parts[1])
				currentTrack = &CUETrack{
					Number:   num,
					DataType: parts[2],
				}
				cue.Tracks = append(cue.Tracks, *currentTrack)
			}

		case strings.HasPrefix(upperLine, "INDEX 01 "):
			if currentTrack != nil {
				lba := parseMSFToLBA(strings.TrimSpace(line[9:]))
				currentTrack.Index01LBA = lba
				// Update the last track in the slice
				if len(cue.Tracks) > 0 {
					cue.Tracks[len(cue.Tracks)-1].Index01LBA = lba
				}
			}

		case strings.HasPrefix(upperLine, "INDEX 00 "):
			if currentTrack != nil {
				currentTrack.Pregap = parseMSFToLBA(strings.TrimSpace(line[9:]))
			}

		case strings.HasPrefix(upperLine, "PREGAP "):
			if currentTrack != nil {
				currentTrack.Pregap = parseMSFToLBA(strings.TrimSpace(line[7:]))
			}

		case strings.HasPrefix(upperLine, "ISRC "):
			if currentTrack != nil {
				currentTrack.ISRC = strings.TrimSpace(line[5:])
			}
		}
	}

	return cue, nil
}

var quotedRe = regexp.MustCompile(`"([^"]*)"`)

func extractQuoted(s string) string {
	s = strings.TrimSpace(s)
	m := quotedRe.FindStringSubmatch(s)
	if len(m) >= 2 {
		return m[1]
	}
	return s
}

func splitCUELine(line string) []string {
	var parts []string
	inQuote := false
	current := strings.Builder{}

	for _, ch := range line {
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func parseMSFToLBA(msf string) int64 {
	parts := strings.Split(msf, ":")
	if len(parts) != 3 {
		return 0
	}
	min, _ := strconv.Atoi(parts[0])
	sec, _ := strconv.Atoi(parts[1])
	frm, _ := strconv.Atoi(parts[2])
	return int64(min*60*75 + sec*75 + frm)
}

func FindBINForCUE(cuePath string, cue *CUESheet) string {
	dir := filepath.Dir(cuePath)

	if len(cue.Tracks) > 0 && cue.Tracks[0].File != "" {
		candidate := filepath.Join(dir, cue.Tracks[0].File)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	base := strings.TrimSuffix(filepath.Base(cuePath), filepath.Ext(cuePath))
	for _, ext := range []string{".bin", ".BIN", ".img", ".IMG"} {
		candidate := filepath.Join(dir, base+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func detectCUEPlatform(tracks []TrackInfo) string {
	hasAudio := false
	hasData := false
	for _, t := range tracks {
		if t.Type == "Audio" {
			hasAudio = true
		} else {
			hasData = true
		}
	}
	if hasAudio && hasData {
		return "Mixed Mode (Audio + Data)"
	}
	if hasAudio && !hasData {
		return "Audio CD"
	}
	return "General"
}
