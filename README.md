# 🔥 NeoBurningGoom

A cross-platform disc burning and image management tool with a beautiful terminal UI.

## Features

- **Analyze disc images** — inspect ISO, CDI, CUE/BIN, XISO, and NRG files
- **Burn images to disc** — write any supported format to CD/DVD
- **Dump discs to image** — read a CD/DVD and save as ISO or BIN/CUE
- **Convert between formats** — CDI → CUE/BIN, ISO → CUE, and more
- **Extract files from ISOs** — pull files out of ISO 9660 images
- **Drive detection** — show optical drive info and media status
- **Erase rewritable discs** — quick or full erase for CD-RW/DVD-RW

### Supported Image Formats

| Format | Platform | Read | Burn | Convert |
|--------|----------|------|------|---------|
| ISO 9660 | General | ✅ | ✅ | → CUE |
| CDI (DiscJuggler) | Sega Dreamcast | ✅ | ✅ | → CUE/BIN |
| CUE/BIN | Audio CD, Mixed Mode | ✅ | ✅ | — |
| XISO | Microsoft Xbox | ✅ | 🔜 | — |
| NRG (Nero) | General | ✅ | ✅ | — |

### Platform Support

| OS | Burn Backend | Drive Detection |
|----|-------------|-----------------|
| macOS | `drutil` (built-in), `xorriso` | `drutil status` |
| Windows | ImgBurn, `cdrdao` | PowerShell / WMI |
| Linux | `xorriso`, `cdrdao` | `xorriso -devices` |

## Installation

```bash
git clone https://github.com/DevSpace88/NeoBurningGoom.git
cd NeoBurningGoom
go build -o neo .
./neo
```

### Prerequisites

**macOS** — everything works out of the box (uses built-in `drutil`).

**Windows** — install [ImgBurn](https://imgburn.com/) for CDI support and general burning.

**Linux** — install `xorriso` and/or `cdrdao`:
```bash
# Debian/Ubuntu
sudo apt install xorriso cdrdao

# Fedora
sudo dnf install xorriso cdrdao

# Arch
sudo pacman -S libburn cdrdao
```

## Usage

Just run it:

```bash
neo
```

You'll get a menu-driven TUI:

```
 🔥 NeoBurningGoom

  🔥 Burn Image          Burn a disc image to CD/DVD
  📀 Dump Disc to Image  Read a CD/DVD and save as ISO or BIN/CUE
  📂 Analyze Image       Read and display details of a disc image file
  💿 Drive Status         Show optical drive info and media status
  🔄 Convert Image        Convert between image formats
  📁 Extract Files        Extract files from an ISO image
  🧹 Erase Disc           Erase a rewritable disc
  ❌ Quit                 Exit NeoBurningGoom
```

### Keybindings

| Key | Action |
|-----|--------|
| `↑`/`↓` | Navigate menu |
| `Enter` | Select / Confirm |
| `ESC` | Go back / Exit |
| `o` | Open file picker |
| `Ctrl+C` | Force quit |

## Architecture

```
internal/
├── image/       Format detection & parsing (ISO, CDI, CUE, XISO, NRG)
├── burner/      Burn engine abstraction (drutil, xorriso, cdrdao, ImgBurn)
├── drive/       Optical drive detection & status
├── convert/     Format conversion & file extraction
└── tui/         Bubble Tea terminal UI
```

The burn backends are abstracted behind a `Burner` interface — each platform picks the best available tool automatically, with fallbacks.

### CDI Format (Dreamcast)

The CDI parser reads DiscJuggler multi-session images commonly used for Dreamcast homebrew and backups. It detects the 2-session layout (Audio + Data), extracts track information, and can convert to CUE/BIN for burning with `cdrdao` or `xorriso`.

## Tech Stack

- **[Go](https://go.dev/)** — because it compiles to a single binary everywhere
- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** — terminal UI framework
- **[Lip Gloss](https://github.com/charmbracelet/lipgloss)** — terminal styling
- **[Bubbles](https://github.com/charmbracelet/bubbles)** — TUI components (spinner, list, text input)

## License

MIT
