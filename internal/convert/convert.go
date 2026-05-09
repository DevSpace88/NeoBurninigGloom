package convert

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	img "NeoBurningGoom/internal/image"
)

// SupportedConversions returns what output formats are available for a given input format.
func SupportedConversions(format img.Format) []string {
	switch format {
	case img.FormatCDI:
		return []string{"CUE/BIN", "ISO"}
	case img.FormatISO:
		return []string{"CUE/BIN"}
	case img.FormatNRG:
		return []string{"CUE/BIN", "ISO"}
	case img.FormatCUE:
		return []string{"ISO"}
	case img.FormatBIN:
		return []string{"ISO"}
	case img.FormatGDI:
		return []string{"CUE/BIN", "ISO"}
	default:
		return nil
	}
}

// Convert converts a source image to the target format.
func Convert(srcPath, outputPath, targetFormat string) (outPaths []string, err error) {
	srcFormat, err := img.DetectFormat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("detect format: %w", err)
	}

	switch targetFormat {
	case "CUE/BIN":
		return convertToCUEBIN(srcPath, outputPath, srcFormat)
	case "ISO":
		return convertToISO(srcPath, outputPath, srcFormat)
	default:
		return nil, fmt.Errorf("unsupported target format: %s", targetFormat)
	}
}

func convertToCUEBIN(srcPath, outputPath string, srcFormat img.Format) ([]string, error) {
	switch srcFormat {
	case img.FormatCDI:
		cue, bin, err := CDIToCUE(srcPath, outputPath)
		if err != nil {
			return nil, err
		}
		return []string{cue, bin}, nil
	case img.FormatISO:
		cue, bin, err := ISOToCUE(srcPath, outputPath)
		if err != nil {
			return nil, err
		}
		return []string{cue, bin}, nil
	case img.FormatGDI:
		return GDIToCUE(srcPath, outputPath)
	default:
		return nil, fmt.Errorf("conversion from %s to CUE/BIN not yet supported", srcFormat)
	}
}

func convertToISO(srcPath, outputPath string, srcFormat img.Format) ([]string, error) {
	switch srcFormat {
	case img.FormatCUE:
		return CUEToISO(srcPath, outputPath)
	case img.FormatBIN:
		return BINToISO(srcPath, outputPath)
	case img.FormatGDI:
		return GDIToISO(srcPath, outputPath)
	default:
		return nil, fmt.Errorf("conversion from %s to ISO not yet supported", srcFormat)
	}
}

func CUEToISO(cuePath, outputPath string) ([]string, error) {
	cue, err := img.ParseCUE(cuePath)
	if err != nil {
		return nil, fmt.Errorf("parse CUE: %w", err)
	}

	binPath := img.FindBINForCUE(cuePath, cue)
	if binPath == "" {
		return nil, fmt.Errorf("BIN file not found for %s", cuePath)
	}

	return binDataToISO(binPath, cue.Tracks, outputPath)
}

func BINToISO(binPath, outputPath string) ([]string, error) {
	return binDataToISO(binPath, nil, outputPath)
}

func binDataToISO(binPath string, tracks []img.CUETrack, outputPath string) ([]string, error) {
	if outputPath == "" {
		outputPath = strings.TrimSuffix(binPath, filepath.Ext(binPath)) + ".iso"
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".iso") {
		outputPath += ".iso"
	}

	binFile, err := os.Open(binPath)
	if err != nil {
		return nil, fmt.Errorf("open BIN: %w", err)
	}
	defer binFile.Close()

	binStat, err := binFile.Stat()
	if err != nil {
		return nil, err
	}

	isoFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create ISO: %w", err)
	}
	defer isoFile.Close()

	// If we have CUE track info, extract only data tracks
	if len(tracks) > 0 {
		for _, track := range tracks {
			if strings.HasPrefix(track.DataType, "MODE1") || strings.HasPrefix(track.DataType, "MODE2") {
				sectorSize := sectorSizeFromMode(track.DataType)
				userDataSize := 2048
				offset := userDataOffset(track.DataType)

				startByte := track.Index01LBA * int64(sectorSize)
				if startByte >= binStat.Size() {
					continue
				}

				if _, err := binFile.Seek(startByte, io.SeekStart); err != nil {
					return nil, fmt.Errorf("seek to track %d: %w", track.Number, err)
				}

				buf := make([]byte, sectorSize)
				for {
					if _, err := io.ReadFull(binFile, buf); err != nil {
						if err == io.ErrUnexpectedEOF || err == io.EOF {
							break
						}
						return nil, fmt.Errorf("read sector: %w", err)
					}
					if _, err := isoFile.Write(buf[offset : offset+userDataSize]); err != nil {
						return nil, fmt.Errorf("write ISO sector: %w", err)
					}
				}
				break
			}
		}
	} else {
		// No CUE info — try to auto-detect sector size
		sectorSize := detectSectorSize(binFile, binStat.Size())

		if sectorSize == 2048 {
			// Already raw user data, just copy
			binFile.Seek(0, io.SeekStart)
			_, err := io.Copy(isoFile, binFile)
			if err != nil {
				return nil, fmt.Errorf("copy: %w", err)
			}
		} else {
			// MODE1/2352 assumed — user data at offset 16
			offset := 16
			userDataSize := 2048

			buf := make([]byte, sectorSize)
			for {
				if _, err := io.ReadFull(binFile, buf); err != nil {
					if err == io.ErrUnexpectedEOF || err == io.EOF {
						break
					}
					return nil, fmt.Errorf("read sector: %w", err)
				}
				if _, err := isoFile.Write(buf[offset : offset+userDataSize]); err != nil {
					return nil, fmt.Errorf("write ISO sector: %w", err)
				}
			}
		}
	}

	return []string{outputPath}, nil
}

func sectorSizeFromMode(mode string) int {
	switch strings.ToUpper(mode) {
	case "MODE1/2048":
		return 2048
	case "MODE1/2352":
		return 2352
	case "MODE2/2336":
		return 2336
	case "MODE2/2352":
		return 2352
	default:
		return 2352
	}
}

func userDataOffset(mode string) int {
	switch strings.ToUpper(mode) {
	case "MODE1/2048":
		return 0
	case "MODE1/2352":
		return 16
	case "MODE2/2336":
		return 8
	case "MODE2/2352":
		return 24
	default:
		return 16
	}
}

func detectSectorSize(f *os.File, fileSize int64) int {
	if fileSize%2352 == 0 && fileSize%2048 != 0 {
		return 2352
	}
	if fileSize%2336 == 0 && fileSize%2048 != 0 {
		return 2336
	}
	return 2048
}

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

	// Generate CUE path — strip any extension, then add .cue
	base := outputPath
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		base = base[:idx]
	}
	cuePath = base + ".cue"
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

	return cuePath, outputPath, nil
}

func ISOToCUE(isoPath, outputPath string) (cuePath string, binPath string, err error) {
	// Copy ISO data to a .bin file and create a matching CUE sheet.
	// ISO is Mode1/2048 — we write a CUE that correctly describes it.

	src, err := os.Open(isoPath)
	if err != nil {
		return "", "", fmt.Errorf("open ISO: %w", err)
	}
	defer src.Close()

	srcStat, err := src.Stat()
	if err != nil {
		return "", "", err
	}

	// Determine BIN output path
	if outputPath == "" {
		outputPath = strings.TrimSuffix(isoPath, filepath.Ext(isoPath)) + ".bin"
	}
	base := outputPath
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		base = base[:idx]
	}
	binPath = base + ".bin"
	cuePath = base + ".cue"

	// Copy ISO → BIN
	binFile, err := os.Create(binPath)
	if err != nil {
		return "", "", fmt.Errorf("create BIN: %w", err)
	}
	defer binFile.Close()

	if _, err := io.Copy(binFile, src); err != nil {
		os.Remove(binPath)
		return "", "", fmt.Errorf("write BIN: %w", err)
	}

	// Build CUE — MODE1/2048 since the ISO data is raw user data (2048 bytes/sector)
	totalSectors := srcStat.Size() / 2048
	indexMSF := lbaToMSF(0)

	binName := filepath.Base(binPath)
	var cue strings.Builder
	cue.WriteString(fmt.Sprintf("FILE \"%s\" BINARY\n", binName))
	cue.WriteString("  TRACK 01 MODE1/2048\n")
	cue.WriteString(fmt.Sprintf("    INDEX 01 %s\n", indexMSF))

	if err := os.WriteFile(cuePath, []byte(cue.String()), 0644); err != nil {
		os.Remove(binPath)
		return "", "", fmt.Errorf("write CUE: %w", err)
	}

	_ = totalSectors
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

// GDI to CUE/BIN converter.
// Merges all GDI track files into a single BIN and writes a CUE sheet.

func GDIToCUE(gdiPath, outputPath string) ([]string, error) {
	gdi, err := img.ParseGDI(gdiPath)
	if err != nil {
		return nil, fmt.Errorf("parse GDI: %w", err)
	}

	if len(gdi.Tracks) == 0 {
		return nil, fmt.Errorf("GDI has no tracks")
	}

	gdiDir := filepath.Dir(gdiPath)

	if outputPath == "" {
		base := strings.TrimSuffix(filepath.Base(gdiPath), filepath.Ext(gdiPath))
		outputPath = filepath.Join(gdiDir, base+".bin")
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".bin") &&
		!strings.HasSuffix(strings.ToLower(outputPath), ".img") {
		outputPath += ".bin"
	}

	base := outputPath
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		base = base[:idx]
	}
	cuePath := base + ".cue"

	binFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create BIN: %w", err)
	}
	defer binFile.Close()

	binName := filepath.Base(outputPath)

	// First pass: calculate track sizes and BIN offsets
	type trackEntry struct {
		track     img.GDITrack
		binOffset int64 // byte offset within merged BIN
		sectors   int64
	}
	var entries []trackEntry
	var binOffset int64

	for _, track := range gdi.Tracks {
		sectors := track.FileSize / int64(track.SectorSize)
		entries = append(entries, trackEntry{track: track, binOffset: binOffset, sectors: sectors})
		binOffset += track.FileSize
	}

	var cue strings.Builder
	cue.WriteString(fmt.Sprintf("FILE \"%s\" BINARY\n", binName))

	for i, entry := range entries {
		mode := "AUDIO"
		if entry.track.Flags == 4 {
			mode = "MODE2/2352"
			if entry.track.SectorSize == 2048 {
				mode = "MODE1/2048"
			}
		}

		cue.WriteString(fmt.Sprintf("  TRACK %02d %s\n", entry.track.Number, mode))

		if i == 0 && entry.track.Flags != 4 {
			cue.WriteString("    PREGAP 00:02:00\n")
		}

		// INDEX 01 = byte offset in BIN converted to MSF using sector size
		sectorOffset := entry.binOffset / int64(entry.track.SectorSize)
		cue.WriteString(fmt.Sprintf("    INDEX 01 %s\n", lbaToMSF(sectorOffset)))

		trackPath := filepath.Join(gdiDir, entry.track.Filename)
		tf, err := os.Open(trackPath)
		if err != nil {
			return nil, fmt.Errorf("open track %d (%s): %w", entry.track.Number, entry.track.Filename, err)
		}
		if _, err := io.Copy(binFile, tf); err != nil {
			tf.Close()
			return nil, fmt.Errorf("copy track %d: %w", entry.track.Number, err)
		}
		tf.Close()
	}

	if err := os.WriteFile(cuePath, []byte(cue.String()), 0644); err != nil {
		return nil, fmt.Errorf("write CUE: %w", err)
	}

	return []string{cuePath, outputPath}, nil
}

// GDI to ISO converter.
// Extracts user data from the data track (usually track 3, LBA 45000).

func GDIToISO(gdiPath, outputPath string) ([]string, error) {
	gdi, err := img.ParseGDI(gdiPath)
	if err != nil {
		return nil, fmt.Errorf("parse GDI: %w", err)
	}

	gdiDir := filepath.Dir(gdiPath)

	var dataTrack *img.GDITrack
	for i := range gdi.Tracks {
		if gdi.Tracks[i].Flags == 4 {
			if dataTrack == nil || gdi.Tracks[i].FileSize > dataTrack.FileSize {
				dataTrack = &gdi.Tracks[i]
			}
		}
	}
	if dataTrack == nil {
		return nil, fmt.Errorf("no data track found in GDI")
	}

	trackPath := filepath.Join(gdiDir, dataTrack.Filename)

	if outputPath == "" {
		base := strings.TrimSuffix(filepath.Base(gdiPath), filepath.Ext(gdiPath))
		outputPath = filepath.Join(gdiDir, base+".iso")
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".iso") {
		outputPath += ".iso"
	}

	trackFile, err := os.Open(trackPath)
	if err != nil {
		return nil, fmt.Errorf("open data track: %w", err)
	}
	defer trackFile.Close()

	isoFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create ISO: %w", err)
	}
	defer isoFile.Close()

	if dataTrack.SectorSize == 2048 {
		_, err = io.Copy(isoFile, trackFile)
		if err != nil {
			return nil, fmt.Errorf("copy: %w", err)
		}
	} else {
		offset := 16
		if dataTrack.SectorSize == 2336 {
			offset = 8
		} else if dataTrack.SectorSize == 2352 {
			offset = 16
		}

		buf := make([]byte, dataTrack.SectorSize)
		for {
			if _, err := io.ReadFull(trackFile, buf); err != nil {
				if err == io.ErrUnexpectedEOF || err == io.EOF {
					break
				}
				return nil, fmt.Errorf("read sector: %w", err)
			}
			if _, err := isoFile.Write(buf[offset : offset+2048]); err != nil {
				return nil, fmt.Errorf("write sector: %w", err)
			}
		}
	}

	return []string{outputPath}, nil
}
