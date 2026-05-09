package drive

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type DumpProgress struct {
	Percent int
	Current int64
	Total   int64
	Phase   string
	Message string
}

type DumpOptions struct {
	DriveIndex int
	DevicePath string // actual /dev/diskN path (not drutil index)
	OutputPath string
	Format     string // "iso", "bin/cue", "raw"
}

func DumpDisc(opts DumpOptions, progress chan<- DumpProgress) error {
	defer close(progress)

	progress <- DumpProgress{Phase: "checking", Message: "Checking disc..."}

	// Check disc status first
	discStatus, err := CheckDisc(opts.DriveIndex)
	if err != nil {
		return fmt.Errorf("check disc: %w", err)
	}

	if !discStatus.HasMedia {
		return fmt.Errorf("no disc in drive — please insert a disc and try again")
	}

	if discStatus.IsBlank {
		return fmt.Errorf("disc is blank — nothing to read. Insert a disc with data")
	}

	progress <- DumpProgress{Phase: "reading", Message: "Reading disc..."}

	switch runtime.GOOS {
	case "darwin":
		return dumpDiscDarwin(opts, progress)
	case "linux":
		return dumpDiscLinux(opts, progress)
	case "windows":
		return dumpDiscWindows(opts, progress)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func dumpDiscDarwin(opts DumpOptions, progress chan<- DumpProgress) error {
	device := opts.DevicePath
	if device == "" {
		device = fmt.Sprintf("/dev/disk%d", opts.DriveIndex)
	}
	rawDevice := strings.Replace(device, "/dev/disk", "/dev/rdisk", 1)

	// Unmount the disc first (required for raw access)
	progress <- DumpProgress{Phase: "unmounting", Message: "Unmounting disc..."}
	if out, err := exec.Command("diskutil", "unmountDisk", device).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to unmount %s: %w\n%s", device, err, string(out))
	}

	switch opts.Format {
	case "iso":
		progress <- DumpProgress{Phase: "reading", Message: "Reading disc with dd..."}
		cmd := exec.Command("dd", "if="+rawDevice, "of="+opts.OutputPath, "bs=2048")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("dd failed: %w\n%s", err, string(out))
		}
		progress <- DumpProgress{Phase: "done", Percent: 100, Message: "Disc dumped successfully"}
		return nil

	case "bin/cue":
		if _, err := exec.LookPath("cdrdao"); err == nil {
			tocPath := strings.TrimSuffix(opts.OutputPath, filepath.Ext(opts.OutputPath)) + ".toc"
			args := []string{
				"read-cd",
				"--device", device,
				"--read-raw",
				"--datafile", opts.OutputPath,
				tocPath,
			}
			progress <- DumpProgress{Phase: "reading", Message: "Reading disc with cdrdao..."}
			cmd := exec.Command("cdrdao", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("cdrdao: %w\n%s", err, string(out))
			}
			progress <- DumpProgress{Phase: "done", Percent: 100, Message: "Disc dumped successfully"}
			return nil
		}
		// Fallback to dd for raw dump
		progress <- DumpProgress{Phase: "reading", Message: "Reading disc with dd (raw mode)..."}
		cmd := exec.Command("dd", "if="+rawDevice, "of="+opts.OutputPath, "bs=2352")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("dd failed: %w\n%s", err, string(out))
		}
		progress <- DumpProgress{Phase: "done", Percent: 100, Message: "Disc dumped successfully"}
		return nil

	default:
		progress <- DumpProgress{Phase: "reading", Message: "Reading disc with dd (raw)..."}
		cmd := exec.Command("dd", "if="+rawDevice, "of="+opts.OutputPath, "bs=2352")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("dd failed: %w\n%s", err, string(out))
		}
		progress <- DumpProgress{Phase: "done", Percent: 100}
		return nil
	}
}

func dumpDiscLinux(opts DumpOptions, progress chan<- DumpProgress) error {
	device := fmt.Sprintf("/dev/sr%d", opts.DriveIndex)

	switch opts.Format {
	case "iso":
		progress <- DumpProgress{Phase: "reading", Message: "Reading disc with dd..."}
		cmd := exec.Command("dd", "if="+device, "of="+opts.OutputPath, "bs=2048", "status=progress")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("dd failed: %w\n%s", err, string(out))
		}
		progress <- DumpProgress{Phase: "done", Percent: 100}
		return nil

	case "bin/cue":
		if _, err := exec.LookPath("cdrdao"); err == nil {
			tocPath := strings.TrimSuffix(opts.OutputPath, filepath.Ext(opts.OutputPath)) + ".toc"
			args := []string{
				"read-cd",
				"--device", device,
				"--read-raw",
				"--datafile", opts.OutputPath,
				tocPath,
			}
			progress <- DumpProgress{Phase: "reading", Message: "Reading disc with cdrdao..."}
			cmd := exec.Command("cdrdao", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("cdrdao: %w\n%s", err, string(out))
			}
			progress <- DumpProgress{Phase: "done", Percent: 100}
			return nil
		}
		progress <- DumpProgress{Phase: "reading", Message: "Reading disc with dd (raw)..."}
		cmd := exec.Command("dd", "if="+device, "of="+opts.OutputPath, "bs=2352", "status=progress")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("dd failed: %w\n%s", err, string(out))
		}
		progress <- DumpProgress{Phase: "done", Percent: 100}
		return nil

	default:
		progress <- DumpProgress{Phase: "reading", Message: "Reading disc with dd..."}
		cmd := exec.Command("dd", "if="+device, "of="+opts.OutputPath, "bs=2048", "status=progress")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("dd failed: %w\n%s", err, string(out))
		}
		progress <- DumpProgress{Phase: "done", Percent: 100}
		return nil
	}
}

func dumpDiscWindows(opts DumpOptions, progress chan<- DumpProgress) error {
	progress <- DumpProgress{Phase: "reading", Message: "Reading disc..."}

	paths := []string{
		"C:\\Program Files (x86)\\ImgBurn\\ImgBurn.exe",
		"C:\\Program Files\\ImgBurn\\ImgBurn.exe",
	}
	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			args := []string{
				"/MODE", "READ",
				"/DEST", opts.OutputPath,
			}
			cmd := exec.Command(p, args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("ImgBurn: %w\n%s", err, string(out))
			}
			progress <- DumpProgress{Phase: "done", Percent: 100}
			return nil
		}
	}

	return fmt.Errorf("no dumping tool found — install ImgBurn")
}
