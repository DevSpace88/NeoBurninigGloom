package drive

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type Drive struct {
	Index       int
	Vendor      string
	Product     string
	Type        string // "CD-ROM", "DVD-ROM", "BD-ROM", "CD/DVD Writer", etc.
	Path        string // device path or drive letter
	HasMedia    bool
	MediaType   string // "CD-R", "CD-RW", "DVD-R", "DVD+R", "DVD-RW", "DVD+RW", "BD-R", etc.
	MediaStatus string // "blank", "appendable", "complete", "unknown"
	Capacity    int64  // in bytes
	UsedSpace   int64  // in bytes
	MaxSpeed    string // e.g., "48x"
}

func Detect() ([]Drive, error) {
	switch runtime.GOOS {
	case "darwin":
		return detectDarwin()
	case "windows":
		return detectWindows()
	case "linux":
		return detectLinux()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func detectDarwin() ([]Drive, error) {
	// Use drutil list to get all optical drives with their indices
	var drives []Drive

	// drutil list gives us the drutil drive numbers
	listOut, err := exec.Command("drutil", "list").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("drutil list: %w\n%s", err, string(listOut))
	}

	for _, line := range strings.Split(string(listOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#"){ 
			continue
		}
		// Parse lines like: "1   PIONEER BD-RW   BDR-XD07U"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		var drutilIdx int
		if _, err := fmt.Sscanf(parts[0], "%d", &drutilIdx); err != nil {
			continue
		}

		drive := Drive{
			Index:   drutilIdx,
			Vendor:  strings.Join(parts[1:], " "),
			Type:    "CD/DVD Writer",
		}

		// Get BSD device path for this drive
		infoOut, err := exec.Command("drutil", "-drive", fmt.Sprintf("%d", drutilIdx), "info").CombinedOutput()
		if err == nil {
			for _, infoLine := range strings.Split(string(infoOut), "\n") {
				infoLine = strings.TrimSpace(infoLine)
				if strings.Contains(infoLine, "IOKitDevicePath") || strings.Contains(infoLine, "/dev/disk") {
					// Extract /dev/diskN from the path
					if idx := strings.Index(infoLine, "/dev/disk"); idx >= 0 {
						drive.Path = infoLine[idx:]
						// Trim anything after the disk number
						for i := len("/dev/disk"); i < len(drive.Path); i++ {
							if drive.Path[i] < '0' || drive.Path[i] > '9' {
								drive.Path = drive.Path[:i]
								break
							}
						}
					}
				}
			}
		}

		// If we didn't find the path from drutil, use diskutil to find it
		if drive.Path == "" {
			drive.Path = findDarwinDiskPath(drutilIdx)
		}

		// Get media status
		statusOut, err := exec.Command("drutil", "-drive", fmt.Sprintf("%d", drutilIdx), "status").CombinedOutput()
		if err == nil {
			for _, sLine := range strings.Split(string(statusOut), "\n") {
				sLine = strings.TrimSpace(sLine)
				if strings.Contains(sLine, "Media Present") {
					drive.HasMedia = !strings.Contains(sLine, "No") && !strings.Contains(sLine, "None") && !strings.Contains(sLine, "false")
				}
			if strings.Contains(sLine, "Media Type") {
				drive.MediaType = strings.TrimSpace(strings.TrimPrefix(sLine, "Media Type:"))
			}
			if strings.Contains(sLine, "Media Status") {
				drive.MediaStatus = strings.TrimSpace(strings.TrimPrefix(sLine, "Media Status:"))
			}
			if strings.Contains(sLine, "Type:") && !strings.Contains(sLine, "Media Type") {
				drive.Type = strings.TrimSpace(strings.TrimPrefix(sLine, "Type:"))
			}
			}
		}

		drives = append(drives, drive)
	}

	return drives, nil
}

// findDarwinDiskPath uses diskutil to find the BSD device path for an optical drive
func findDarwinDiskPath(drutilIdx int) string {
	out, err := exec.Command("diskutil", "list", "external").CombinedOutput()
	if err != nil {
		return fmt.Sprintf("/dev/disk%d", drutilIdx+1)
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "/dev/disk") && (strings.Contains(line, "CD") || strings.Contains(line, "DVD") || strings.Contains(line, "Optical")) {
			if idx := strings.Index(line, "/dev/disk"); idx >= 0 {
				return line[idx:]
			}
		}
	}

	// Fallback: find any external disk that isn't disk0 (internal SSD)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "/dev/disk"); idx >= 0 {
			dev := line[idx:]
			if dev != "/dev/disk0" {
				// Could be our optical drive
				// Verify with diskutil info
				infoOut, err := exec.Command("diskutil", "info", dev).CombinedOutput()
				if err == nil && (strings.Contains(string(infoOut), "Optical") || strings.Contains(string(infoOut), "CD") || strings.Contains(string(infoOut), "DVD")) {
					return dev
				}
			}
		}
	}

	return fmt.Sprintf("/dev/disk%d", drutilIdx+1)
}

func detectWindows() ([]Drive, error) {
	// On Windows, use wmic or PowerShell to find CD/DVD drives
	out, err := exec.Command("powershell", "-Command",
		"Get-CimInstance Win32_CDROMDrive | Select-Object Drive, Name, MediaLoaded, MediaType, Caption | ConvertTo-Json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("powershell: %w", err)
	}

	var drives []Drive
	lines := strings.Split(string(out), "\n")
	drive := Drive{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "\"Drive\":"):
			drive.Path = extractJSONValue(line)
		case strings.Contains(line, "\"Caption\":"):
			drive.Product = extractJSONValue(line)
		case strings.Contains(line, "\"MediaLoaded\":"):
			drive.HasMedia = strings.Contains(strings.ToLower(line), "true")
		case strings.Contains(line, "\"MediaType\":"):
			drive.MediaType = extractJSONValue(line)
		}
		if strings.Contains(line, "}") && (drive.Path != "" || drive.Product != "") {
			drive.Index = len(drives)
			drives = append(drives, drive)
			drive = Drive{}
		}
	}

	return drives, nil
}

func detectLinux() ([]Drive, error) {
	// Use cdrdao or xorriso to detect drives
	out, err := exec.Command("xorriso", "-devices").CombinedOutput()
	if err != nil {
		// Fallback: look at /dev/sr* and /dev/cdrom
		return detectLinuxSysfs()
	}

	var drives []Drive
	lines := strings.Split(string(out), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "/dev/sr") || strings.Contains(line, "/dev/cdrom") {
			parts := strings.Fields(line)
			drive := Drive{
				Index: len(drives),
			}
			for _, p := range parts {
				if strings.HasPrefix(p, "/dev/") {
					drive.Path = p
				}
			}
			if idx := strings.Index(line, "'"); idx >= 0 {
				end := strings.Index(line[idx+1:], "'")
				if end >= 0 {
					drive.Product = line[idx+1 : idx+1+end]
				}
			}
			drives = append(drives, drive)
		}
	}

	return drives, nil
}

func detectLinuxSysfs() ([]Drive, error) {
	// Minimal detection via /sys/bus/scsi
	out, err := exec.Command("ls", "/dev/sr*").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("no optical drives found")
	}

	var drives []Drive
	for _, dev := range strings.Fields(string(out)) {
		drives = append(drives, Drive{
			Index:   len(drives),
			Path:    dev,
			Product: "Optical Drive",
		})
	}

	return drives, nil
}

func extractJSONValue(line string) string {
	line = strings.TrimSpace(line)
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	val := strings.TrimSpace(parts[1])
	val = strings.Trim(val, "\",")
	return val
}

// Eject opens the tray of the specified drive.
func Eject(driveIdx int) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("drutil", "tray", "eject", fmt.Sprintf("%d", driveIdx)).Run()
	case "linux":
		return exec.Command("eject").Run()
	case "windows":
		return exec.Command("powershell", "-Command",
			"$wmp = New-Object -ComObject WMPlayer.OCX; $wmp.cdromCollection.Item(0).Eject()").Run()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

type DiscStatus struct {
	HasMedia    bool
	IsBlank     bool
	IsWritable  bool
	MediaType   string // "CD-R", "CD-RW", "DVD-R", "DVD+R", etc.
	MediaStatus string // "blank", "appendable", "complete", "unknown"
	Capacity    int64  // in bytes
	ContentName string // volume name if disc has data
}

func CheckDisc(driveIdx int) (*DiscStatus, error) {
	switch runtime.GOOS {
	case "darwin":
		return checkDiscDarwin(driveIdx)
	case "linux":
		return checkDiscLinux(driveIdx)
	case "windows":
		return checkDiscWindows(driveIdx)
	default:
		return nil, fmt.Errorf("unsupported platform")
	}
}

func checkDiscDarwin(driveIdx int) (*DiscStatus, error) {
	status := &DiscStatus{}

	// Try drutil with drive index first, then without
	var out []byte
	out, err := exec.Command("drutil", "-drive", fmt.Sprintf("%d", driveIdx), "status").CombinedOutput()
	if err != nil {
		// Fallback: try without drive index (uses first optical drive)
		out, err = exec.Command("drutil", "status").CombinedOutput()
		if err != nil {
			// Can't check disc status — don't block, return what we have
			status.HasMedia = true
			return status, nil
		}
	}

	output := string(out)

	// drutil status output has two formats:
	// Format 1 (old): "Media Present: Yes", "Media Type: CD-R", "Media Status: blank"
	// Format 2 (new): "Type: DVD-R", "Name: /dev/disk4", "Writability: appendable, blank"

	if strings.Contains(output, "Media Present") {
		// Old format
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "Media Present") {
				status.HasMedia = !strings.Contains(line, "No") && !strings.Contains(line, "None") && !strings.Contains(line, "false")
			}
			if strings.Contains(line, "Media Type") && !strings.Contains(line, "Book Type") {
				status.MediaType = strings.TrimSpace(strings.TrimPrefix(line, "Media Type:"))
			}
			if strings.Contains(line, "Media Status") {
				status.MediaStatus = strings.TrimSpace(strings.TrimPrefix(line, "Media Status:"))
				status.IsBlank = strings.Contains(strings.ToLower(status.MediaStatus), "blank")
			}
		}
	} else {
		// New format — disc is present if we got any Type: line with media info
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Type:") {
				typeVal := strings.TrimSpace(strings.TrimPrefix(line, "Type:"))
				if typeVal != "" {
					status.HasMedia = true
					status.MediaType = typeVal
					for _, w := range []string{"CD-R", "CD-RW", "DVD-R", "DVD+R", "DVD-RW", "DVD+RW", "DVD-R DL", "DVD+R DL", "BD-R", "BD-RE"} {
						if strings.Contains(typeVal, w) {
							status.IsWritable = true
							break
						}
					}
				}
			}
			if strings.HasPrefix(line, "Name:") {
				// "Name: /dev/disk4"
				nameVal := strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
				if strings.HasPrefix(nameVal, "/dev/") {
					status.Capacity = 0
				}
			}
			if strings.HasPrefix(line, "Writability:") {
				writVal := strings.TrimSpace(strings.TrimPrefix(line, "Writability:"))
				status.MediaStatus = writVal
				status.IsBlank = strings.Contains(strings.ToLower(writVal), "blank")
				if strings.Contains(writVal, "appendable") || strings.Contains(writVal, "overwritable") {
					status.IsWritable = true
				}
			}
			if strings.HasPrefix(line, "Space Used:") {
				if strings.Contains(line, "0.00MB") || strings.Contains(line, "blocks:        0") {
					status.IsBlank = true
				}
			}
		}
	}

	return status, nil
}

func checkDiscLinux(driveIdx int) (*DiscStatus, error) {
	status := &DiscStatus{}

	out, err := exec.Command("cdrdao", "disk-info", "--device", fmt.Sprintf("/dev/sr%d", driveIdx)).CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "CD-RW") || strings.Contains(line, "DVD-RW") {
				status.IsWritable = true
			}
			if strings.Contains(line, "blank") {
				status.IsBlank = true
			}
		}
		status.HasMedia = true
	}

	return status, nil
}

func checkDiscWindows(driveIdx int) (*DiscStatus, error) {
	return &DiscStatus{HasMedia: false}, nil
}
