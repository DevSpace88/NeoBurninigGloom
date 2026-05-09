package image

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GDI (GigaByte Disc Image) format parser for Dreamcast GD-ROM images.
//
// A .gdi file is a plain-text track descriptor, similar to a CUE sheet.
// Format:
//   Line 1: track count
//   Lines 2..N: <trackNum> <startLBA> <flags> <sectorSize> "<filename>" <unknown>
//
// Typical Dreamcast GDI layout:
//   Track 1: Audio (low-density area, LBA 0)
//   Track 2: Audio (optional, IP.BIN intro)
//   Track 3: Data (high-density area, LBA 45000)

type GDITrack struct {
	Number    int
	StartLBA  int64
	Flags     int
	SectorSize int
	Filename  string
	FileSize  int64
}

type GDIInfo struct {
	Tracks  []GDITrack
	Dir     string
}

func analyzeGDI(info *ImageInfo) (*ImageInfo, error) {
	gdi, err := ParseGDI(info.Path)
	if err != nil {
		return nil, fmt.Errorf("parse GDI: %w", err)
	}

	var tracks []TrackInfo
	var totalSize int64

	for _, gt := range gdi.Tracks {
		trackType := "Audio"
		if gt.Flags == 4 {
			trackType = "Mode2/Form1"
			if gt.SectorSize == 2048 {
				trackType = "Mode1"
			}
		}

		sectors := gt.FileSize / int64(gt.SectorSize)

		tracks = append(tracks, TrackInfo{
			Number:    gt.Number,
			Type:      trackType,
			StartLBA:  gt.StartLBA,
			Sectors:   sectors,
			SizeBytes: gt.FileSize,
		})
		totalSize += gt.FileSize
	}

	info.Tracks = tracks
	info.Size = totalSize
	info.Platform = "Sega Dreamcast"
	info.Sessions = 2

	if info.RawDetails == nil {
		info.RawDetails = make(map[string]string)
	}
	info.RawDetails["Track Count"] = fmt.Sprintf("%d", len(gdi.Tracks))

	dataTracks := 0
	audioTracks := 0
	for _, t := range tracks {
		if t.Type == "Audio" {
			audioTracks++
		} else {
			dataTracks++
		}
	}
	info.RawDetails["Audio Tracks"] = fmt.Sprintf("%d", audioTracks)
	info.RawDetails["Data Tracks"] = fmt.Sprintf("%d", dataTracks)

	return info, nil
}

func ParseGDI(path string) (*GDIInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dir := filepath.Dir(path)
	gdi := &GDIInfo{Dir: dir}

	scanner := bufio.NewScanner(f)
	lineNum := 0
	trackCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNum++

		if lineNum == 1 {
			trackCount, err = strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("invalid GDI header: %q", line)
			}
			continue
		}

		fields := splitGDIFields(line)
		if len(fields) < 6 {
			return nil, fmt.Errorf("invalid GDI track line %d: expected 6 fields, got %d", lineNum, len(fields))
		}

		trackNum, _ := strconv.Atoi(fields[0])
		startLBA, _ := strconv.ParseInt(fields[1], 10, 64)
		flags, _ := strconv.Atoi(fields[2])
		sectorSize, _ := strconv.Atoi(fields[3])
		filename := fields[4]

		// Resolve filename relative to GDI directory
		trackPath := filepath.Join(dir, filename)
		var fileSize int64
		if stat, err := os.Stat(trackPath); err == nil {
			fileSize = stat.Size()
		}

		gdi.Tracks = append(gdi.Tracks, GDITrack{
			Number:     trackNum,
			StartLBA:   startLBA,
			Flags:      flags,
			SectorSize: sectorSize,
			Filename:   filename,
			FileSize:   fileSize,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if trackCount > 0 && len(gdi.Tracks) != trackCount {
		return nil, fmt.Errorf("GDI track count mismatch: header says %d, found %d", trackCount, len(gdi.Tracks))
	}

	return gdi, nil
}

func splitGDIFields(line string) []string {
	var fields []string
	inQuote := false
	current := strings.Builder{}

	for _, ch := range line {
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}

func GDITrackPath(gdiPath string, trackFilename string) string {
	return filepath.Join(filepath.Dir(gdiPath), trackFilename)
}
