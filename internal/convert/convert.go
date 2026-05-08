package convert

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	img "NeoBurningGoom/internal/image"
)

// CDI to CUE/BIN converter.
// Extracts track layout from CDI and generates a CUE sheet + raw BIN file.

func CDIToCUE(cdiPath, outputPath string) (cuePath string, binPath string, err error) {
	info, err := img.Analyze(cdiPath)
	if err != nil {
		return "", "", fmt.Errorf("analyze CDI: %w", err)
	}
	if info.Format != img.FormatCDI {
		return "", "", fmt.Errorf("not a CDI file")
	}

	// Open source CDI
	src, err := os.Open(cdiPath)
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	// Create BIN output
	if outputPath == "" {
		outputPath = strings.TrimSuffix(cdiPath, ".cdi") + ".bin"
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".bin") &&
		!strings.HasSuffix(strings.ToLower(outputPath), ".img") {
		outputPath += ".bin"
	}

	binFile, err := os.Create(outputPath)
	if err != nil {
		return "", "", fmt.Errorf("create BIN: %w", err)
	}
	defer binFile.Close()

	// Generate CUE path
	cuePath = strings.TrimSuffix(outputPath, ".bin") + ".cue"
	if strings.HasSuffix(strings.ToLower(outputPath), ".img") {
		cuePath = strings.TrimSuffix(outputPath, ".img") + ".cue"
	}
	binName := outputPath
	if strings.Contains(binName, "/") || strings.Contains(binName, "\\") {
		parts := strings.Split(binName, "/")
		if ws := strings.Split(binName, "\\"); len(ws) > len(parts) {
			parts = ws
		}
		binName = parts[len(parts)-1]
	}

	// Build CUE content
	var cue strings.Builder
	cue.WriteString(fmt.Sprintf("FILE \"%s\" BINARY\n", binName))

	for i, track := range info.Tracks {
		mode := "AUDIO"
		if track.Type != "Audio" {
			mode = "MODE2/2352"
			if strings.HasPrefix(track.Type, "Mode1") {
				mode = "MODE1/2048"
			}
		}

		cue.WriteString(fmt.Sprintf("  TRACK %02d %s\n", track.Number, mode))

		// Add pregap for track 1 if audio (2 seconds)
		if i == 0 && track.Type == "Audio" {
			cue.WriteString("    PREGAP 00:02:00\n")
		}

		indexMSF := lbaToMSF(track.StartLBA)
		cue.WriteString(fmt.Sprintf("    INDEX 01 %s\n", indexMSF))
	}

	// Write raw disc data to BIN
	if _, err := io.Copy(binFile, src); err != nil {
		return "", "", fmt.Errorf("write BIN: %w", err)
	}

	// Write CUE
	if err := os.WriteFile(cuePath, []byte(cue.String()), 0644); err != nil {
		return "", "", fmt.Errorf("write CUE: %w", err)
	}

	return cuePath, binPath, nil
}

func ISOToCUE(isoPath, outputPath string) (cuePath string, binPath string, err error) {
	if outputPath == "" {
		outputPath = strings.TrimSuffix(isoPath, ".iso")
	}

	cuePath = outputPath + ".cue"
	binPath = isoPath // the ISO itself is the data source

	binName := isoPath
	if strings.Contains(binName, "/") || strings.Contains(binName, "\\") {
		parts := strings.Split(binName, "/")
		if ws := strings.Split(binName, "\\"); len(ws) > len(parts) {
			parts = ws
		}
		binName = parts[len(parts)-1]
	}

	var cue strings.Builder
	cue.WriteString(fmt.Sprintf("FILE \"%s\" BINARY\n", binName))
	cue.WriteString("  TRACK 01 MODE1/2048\n")
	cue.WriteString("    INDEX 01 00:00:00\n")

	if err := os.WriteFile(cuePath, []byte(cue.String()), 0644); err != nil {
		return "", "", err
	}

	return cuePath, binPath, nil
}

func ExtractFilesFromISO(isoPath, destDir string) error {
	f, err := os.Open(isoPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Read primary volume descriptor at sector 16 (offset 0x8000)
	buf := make([]byte, 2048)
	if _, err := f.Seek(0x8000, io.SeekStart); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	if string(buf[1:6]) != "CD001" {
		return fmt.Errorf("not a valid ISO 9660")
	}

	// Root directory record starts at offset 156 in the PVD
	rootRecord := buf[156:190]
	rootLBA := int64(rootRecord[2]) | int64(rootRecord[3])<<8 | int64(rootRecord[4])<<16 | int64(rootRecord[5])<<24
	rootLen := int64(rootRecord[10]) | int64(rootRecord[11])<<8 | int64(rootRecord[12])<<16 | int64(rootRecord[13])<<24

	return extractISODirectory(f, rootLBA, rootLen, destDir)
}

func extractISODirectory(f *os.File, lba, length int64, destDir string) error {
	buf := make([]byte, length)
	if _, err := f.Seek(lba*2048, io.SeekStart); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	offset := int64(0)
	for offset < length {
		recLen := int64(buf[offset])
		if recLen == 0 {
			// Padding, skip to next sector
			offset = (offset/2048 + 1) * 2048
			continue
		}

		nameLen := int64(buf[offset+32])
		isDir := buf[offset+25]&0x02 != 0
		name := string(buf[offset+33 : offset+33+nameLen])

		fileLBA := int64(buf[offset+2]) | int64(buf[offset+3])<<8 | int64(buf[offset+4])<<16 | int64(buf[offset+5])<<24
		fileLen := int64(buf[offset+10]) | int64(buf[offset+11])<<8 | int64(buf[offset+12])<<16 | int64(buf[offset+13])<<24

		// Skip . and .. entries
		if name == "\x00" || name == "\x01" {
			offset += recLen
			continue
		}

		// Strip version number (;1)
		if idx := strings.Index(name, ";"); idx >= 0 {
			name = name[:idx]
		}

		if isDir {
			subDir := destDir + "/" + name
			if err := os.MkdirAll(subDir, 0755); err != nil {
				return err
			}
			if err := extractISODirectory(f, fileLBA, fileLen, subDir); err != nil {
				return err
			}
		} else {
			if err := extractISOFile(f, fileLBA, fileLen, destDir+"/"+name); err != nil {
				return err
			}
		}

		offset += recLen
	}

	return nil
}

func extractISOFile(f *os.File, lba, length int64, destPath string) error {
	if _, err := f.Seek(lba*2048, io.SeekStart); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.CopyN(out, f, length)
	return err
}

// Parse a CUE file and return its content as lines (useful for display).
func ReadCUEContent(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func lbaToMSF(lba int64) string {
	lba += 150 // add 2-second pregap offset
	min := lba / (75 * 60)
	sec := (lba / 75) % 60
	frm := lba % 75
	return fmt.Sprintf("%02d:%02d:%02d", min, sec, frm)
}
