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
	// drutil status gives drive info on macOS
	out, err := exec.Command("drutil", "status").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("drutil: %w\n%s", err, string(out))
	}

	var drives []Drive
	drive := Drive{Index: 0}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "Vendor"):
			parts := strings.SplitN(line, "Product:", 2)
			if len(parts) >= 1 {
				drive.Vendor = strings.TrimSpace(strings.TrimPrefix(parts[0], "Vendor:"))
			}
			if len(parts) >= 2 {
				drive.Product = strings.TrimSpace(parts[1])
			}
		case strings.Contains(line, "Type:"):
			drive.Type = strings.TrimSpace(strings.TrimPrefix(line, "Type:"))
		case strings.Contains(line, "Media Present"):
			drive.HasMedia = strings.Contains(line, "Yes") || strings.Contains(line, "true")
		case strings.Contains(line, "Media Type"):
			drive.MediaType = strings.TrimSpace(strings.TrimPrefix(line, "Media Type:"))
		case strings.Contains(line, "Media Status"):
			drive.MediaStatus = strings.TrimSpace(strings.TrimPrefix(line, "Media Status:"))
		}
	}

	if drive.Vendor != "" || drive.Product != "" {
		drives = append(drives, drive)
	}

	// If drutil didn't find anything, try drutil info
	if len(drives) == 0 {
		out, err := exec.Command("drutil", "info").CombinedOutput()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			drive := Drive{Index: 0}
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.Contains(line, "IOKitClass") || strings.Contains(line, "IOKitMatch") {
					continue
				}
				if strings.Contains(line, "Vendor:") {
					drive.Vendor = strings.TrimSpace(strings.TrimPrefix(line, "Vendor:"))
				}
				if strings.Contains(line, "Product:") {
					drive.Product = strings.TrimSpace(strings.TrimPrefix(line, "Product:"))
				}
			}
			if drive.Vendor != "" || drive.Product != "" {
				drives = append(drives, drive)
			}
		}
	}

	return drives, nil
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
