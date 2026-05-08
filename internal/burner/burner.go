package burner

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

type BurnOptions struct {
	Speed      int    // 0 = max, otherwise 1, 2, 4, 8, 16, 24, 48, etc.
	DryRun     bool   // simulate only
	EjectAfter bool   // eject disc after burning
	Verify     bool   // verify after burning
	DriveIndex int    // which drive to use
	ImagePath  string // path to image file
}

type BurnProgress struct {
	Percent   int
	Speed     string
	Buffer    int
	Current   int64 // bytes written
	Total     int64 // total bytes
	Phase     string // "writing", "fixating", "verifying", "done", "error"
	Message   string
}

type Burner interface {
	Burn(opts BurnOptions, progress chan<- BurnProgress) error
	Erase(driveIndex int, quick bool) error
	Available() bool
	Name() string
}

func New() Burner {
	switch runtime.GOOS {
	case "darwin":
		return &drutilBurner{}
	case "windows":
		return newImgBurnBurner()
	case "linux":
		return &xorrisoBurner{}
	default:
		return &xorrisoBurner{}
	}
}

func AllBackends() []Burner {
	var backends []Burner
	switch runtime.GOOS {
	case "darwin":
		backends = append(backends, &drutilBurner{})
		backends = append(backends, &xorrisoBurner{})
	case "windows":
		backends = append(backends, newImgBurnBurner())
		backends = append(backends, &cdrdaoBurner{})
	case "linux":
		backends = append(backends, &xorrisoBurner{})
		backends = append(backends, &cdrdaoBurner{})
	}
	return backends
}

// --- drutil (macOS) ---

type drutilBurner struct{}

func (b *drutilBurner) Name() string { return "drutil (macOS)" }

func (b *drutilBurner) Available() bool {
	_, err := exec.LookPath("drutil")
	return err == nil
}

func (b *drutilBurner) Burn(opts BurnOptions, progress chan<- BurnProgress) error {
	defer close(progress)

	args := []string{"burn", opts.ImagePath}
	if opts.Speed > 0 {
		args = append(args, "-speed", fmt.Sprintf("%d", opts.Speed))
	}
	if opts.DryRun {
		args = append(args, "-simulate")
	}
	if !opts.EjectAfter {
		args = append(args, "-notray")
	}

	cmd := exec.Command("drutil", args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		progress <- BurnProgress{Phase: "error", Message: err.Error()}
		return err
	}

	// Parse progress from stdout
	go parseDrutilProgress(stdout, progress)
	go io.Copy(io.Discard, stderr)

	err := cmd.Wait()
	if err != nil {
		progress <- BurnProgress{Phase: "error", Message: err.Error()}
		return err
	}

	progress <- BurnProgress{Phase: "done", Percent: 100}
	return nil
}

func (b *drutilBurner) Erase(driveIndex int, quick bool) error {
	args := []string{"erase", "disc"}
	if quick {
		args = []string{"erase", "quick"}
	}
	return exec.Command("drutil", args...).Run()
}

func parseDrutilProgress(r io.Reader, progress chan<- BurnProgress) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if err != nil {
			return
		}
		line := strings.TrimSpace(string(buf[:n]))
		progress <- BurnProgress{
			Phase:   "writing",
			Message: line,
		}
	}
}

// --- xorriso (cross-platform, Linux primary) ---

type xorrisoBurner struct{}

func (b *xorrisoBurner) Name() string { return "xorriso" }

func (b *xorrisoBurner) Available() bool {
	_, err := exec.LookPath("xorriso")
	return err == nil
}

func (b *xorrisoBurner) Burn(opts BurnOptions, progress chan<- BurnProgress) error {
	defer close(progress)

	args := []string{
		"-as", "cdrecord",
		fmt.Sprintf("dev=/dev/sr%d", opts.DriveIndex),
		"-v",
	}
	if opts.Speed > 0 {
		args = append(args, fmt.Sprintf("speed=%d", opts.Speed))
	}
	if opts.DryRun {
		args = append(args, "-dummy")
	}
	args = append(args, opts.ImagePath)

	cmd := exec.Command("xorriso", args...)
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		progress <- BurnProgress{Phase: "error", Message: err.Error()}
		return err
	}

	go parseCdrecordProgress(stderr, progress)

	err := cmd.Wait()
	if err != nil {
		progress <- BurnProgress{Phase: "error", Message: err.Error()}
		return err
	}

	progress <- BurnProgress{Phase: "done", Percent: 100}
	return nil
}

func (b *xorrisoBurner) Erase(driveIndex int, quick bool) error {
	args := []string{
		"-as", "cdrecord",
		fmt.Sprintf("dev=/dev/sr%d", driveIndex),
	}
	if quick {
		args = append(args, "blank=fast")
	} else {
		args = append(args, "blank=all")
	}
	return exec.Command("xorriso", args...).Run()
}

// --- cdrdao (cross-platform, needed for CDI/Audio) ---

type cdrdaoBurner struct{}

func (b *cdrdaoBurner) Name() string { return "cdrdao" }

func (b *cdrdaoBurner) Available() bool {
	_, err := exec.LookPath("cdrdao")
	return err == nil
}

func (b *cdrdaoBurner) Burn(opts BurnOptions, progress chan<- BurnProgress) error {
	defer close(progress)

	args := []string{
		"write",
		"--device", fmt.Sprintf("/dev/sr%d", opts.DriveIndex),
		"--buffers", "64",
	}
	if opts.Speed > 0 {
		args = append(args, "--speed", fmt.Sprintf("%d", opts.Speed))
	}
	if opts.DryRun {
		args = append(args, "--simulate")
	}
	args = append(args, opts.ImagePath)

	cmd := exec.Command("cdrdao", args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		progress <- BurnProgress{Phase: "error", Message: err.Error()}
		return err
	}

	go parseCdrecordProgress(stdout, progress)
	go io.Copy(io.Discard, stderr)

	err := cmd.Wait()
	if err != nil {
		progress <- BurnProgress{Phase: "error", Message: err.Error()}
		return err
	}

	progress <- BurnProgress{Phase: "done", Percent: 100}
	return nil
}

func (b *cdrdaoBurner) Erase(driveIndex int, quick bool) error {
	args := []string{
		"blank",
		"--device", fmt.Sprintf("/dev/sr%d", driveIndex),
	}
	if quick {
		args = append(args, "--blank-mode", "minimal")
	}
	return exec.Command("cdrdao", args...).Run()
}

// --- ImgBurn (Windows) ---

type imgburnBurner struct {
	exePath string
}

func newImgBurnBurner() *imgburnBurner {
	// Check common ImgBurn install locations
	paths := []string{
		"C:\\Program Files (x86)\\ImgBurn\\ImgBurn.exe",
		"C:\\Program Files\\ImgBurn\\ImgBurn.exe",
	}
	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return &imgburnBurner{exePath: p}
		}
	}
	// Try PATH
	if p, err := exec.LookPath("ImgBurn.exe"); err == nil {
		return &imgburnBurner{exePath: p}
	}
	return &imgburnBurner{}
}

func (b *imgburnBurner) Name() string { return "ImgBurn" }

func (b *imgburnBurner) Available() bool {
	return b.exePath != ""
}

func (b *imgburnBurner) Burn(opts BurnOptions, progress chan<- BurnProgress) error {
	defer close(progress)

	args := []string{
		"/MODE", "WRITE",
		"/SRC", opts.ImagePath,
	}
	if opts.Speed > 0 {
		args = append(args, "/SPEED", fmt.Sprintf("%d", opts.Speed))
	}
	if opts.Verify {
		args = append(args, "/VERIFY")
	}
	if opts.EjectAfter {
		args = append(args, "/EJECT")
	}
	if opts.DryRun {
		args = append(args, "/TESTMODE")
	}

	cmd := exec.Command(b.exePath, args...)
	err := cmd.Run()
	if err != nil {
		progress <- BurnProgress{Phase: "error", Message: err.Error()}
		return err
	}

	progress <- BurnProgress{Phase: "done", Percent: 100}
	return nil
}

func (b *imgburnBurner) Erase(driveIndex int, quick bool) error {
	args := []string{
		"/MODE", "ERASE",
	}
	if quick {
		args = append(args, "/TYPE", "QUICK")
	} else {
		args = append(args, "/TYPE", "FULL")
	}
	return exec.Command(b.exePath, args...).Run()
}

// --- Progress parsing helpers ---

func parseCdrecordProgress(r io.Reader, progress chan<- BurnProgress) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if err != nil {
			return
		}
		line := strings.TrimSpace(string(buf[:n]))
		lines := strings.Split(line, "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}

			p := BurnProgress{Phase: "writing", Message: l}

			// cdrecord-style progress: "Track 01:  42 of  100 MB written (fifo 100%) [buf  98%]  12.3x."
			if strings.Contains(l, "written") || strings.Contains(l, "%") {
				// Try to extract percentage
				for _, part := range strings.Fields(l) {
					part = strings.TrimSuffix(part, "%")
					if pct, err := fmt.Sscanf(part, "%d"); err == nil && pct >= 0 && pct <= 100 {
						p.Percent = pct
						break
					}
				}
			}

			if strings.Contains(l, "Fixating") {
				p.Phase = "fixating"
			}
			if strings.Contains(l, "Writing") {
				p.Phase = "writing"
			}

			progress <- p
		}
	}
}
