# SQDoc v1

SQDoc is built in Go with a custom-rendered, cross-platform desktop architecture.

## Current Status

This repository contains:
- SQDoc v1 binary format core (`pkg/sqdoc`) with validation, encode/decode, load/save.
- Test coverage for roundtrip, corruption detection, random-access flag checks, and validation errors.
- Interactive block-based editor runtime with mouse/keyboard editing, selection, and clipboard operations.
- Formatting directive support (bold/italic/underline/font size/color) persisted outside text payloads.
- Build scripts for Windows and Linux.

## Build

### Windows
```bat
scripts\build_windows.bat
```

### Linux
```bash
./scripts/build_linux.sh
```

## Test

```bash
go test ./...
```

## Run

```bash
# Windows
build\windows\side.exe

# Linux
./build/linux/side
```

## Notes

Current editor controls:
- `Ctrl+N`: New document
- `Ctrl+O`: Open via file explorer dialog
- `Ctrl+S`: Save (opens save dialog for untitled docs)
- `Ctrl+Shift+S`: Save As via file explorer dialog
- Top menu buttons (`New/Open/Save/Save As/Undo/Redo/A-/A+/Help`) are clickable
- `Ctrl+Z` / `Ctrl+Y`: Undo / Redo
- `Ctrl+P`: Toggle block map side panel
- `F1`: Toggle help overlay
- Mouse click/drag: Caret placement and text selection
- `Ctrl+C` / `Ctrl+X` / `Ctrl+V`: Copy / Cut / Paste
- Mouse wheel / `PageUp` / `PageDown`: Vertical scrolling
- `Shift + Mouse wheel`: Horizontal scrolling
- `Enter`: Split current block
- `Backspace` / `Delete`: Delete/merge text across blocks
- `Arrow Up/Down`: Switch blocks
- `Arrow Left/Right`, `Home`, `End`: Caret movement
- `Ctrl+Left/Right`: Word-wise caret movement
- `Alt+Left/Right`: Jump to block start/end
- `Ctrl+B`, `Ctrl+I`, `Ctrl+U`: Bold / Italic / Underline
- `Ctrl+.` / `Ctrl+,`: Increase / Decrease font size
- `Ctrl+Shift+C`: Cycle block color
- `Ctrl +` / `Ctrl -`: UI scaling levels
