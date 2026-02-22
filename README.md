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
- Encrypted open now triggers an in-app password popup prompt
- `Ctrl+S`: Save (opens save dialog for untitled docs)
- `Ctrl+Shift+S`: Save As via file explorer dialog
- Top menu buttons (`New/Open/Save/Save As/Undo/Redo/Data Map/Encryption/A-/A+/Help`) are clickable
- `Ctrl+Z` / `Ctrl+Y`: Undo / Redo
- `Ctrl+P`: Toggle block map side panel
- `Ctrl+E`: Toggle encryption view
- `F1`: Toggle help overlay
- Mouse click/drag: Caret placement and text selection
- `Ctrl+C` / `Ctrl+X` / `Ctrl+V`: Copy / Cut / Paste
- Mouse wheel / `PageUp` / `PageDown`: Vertical scrolling
- `Shift + Mouse wheel`: Horizontal scrolling
- `Enter`: Split current block
- `Backspace` / `Delete`: Delete/merge text across blocks
- `Ctrl+Backspace` / `Ctrl+Delete`: Delete previous/next word
- `Arrow Up/Down`: Switch blocks
- `Arrow Left/Right`, `Home`, `End`: Caret movement
- `Ctrl+Left/Right`: Word-wise caret movement
- `Alt+Left/Right`: Jump to block start/end
- `Ctrl+B`, `Ctrl+I`, `Ctrl+U`: Bold / Italic / Underline
- `Ctrl+Shift+H`: Toggle highlight
- `Ctrl+.` / `Ctrl+,`: Increase / Decrease font size
- Toolbar controls are clickable for `Bold/Italic/Underline/Highlight`, font step/input, and color picker
- `Ctrl+Shift+C`: Cycle block color (keyboard shortcut)
- `Ctrl +` / `Ctrl -`: UI scaling levels
