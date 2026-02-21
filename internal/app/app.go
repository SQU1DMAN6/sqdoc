package app

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"sqdoc/internal/editor"
	"sqdoc/internal/render"
	"sqdoc/internal/ui"
	"sqdoc/pkg/sqdoc"

	"github.com/atotto/clipboard"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/sqweek/dialog"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

type snapshot struct {
	doc          *sqdoc.Document
	currentBlock int
	caretByte    int
}

type rect struct {
	x int
	y int
	w int
	h int
}

func (r rect) contains(x, y int) bool {
	return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h
}

type actionButton struct {
	id     string
	label  string
	r      rect
	active bool
}

type colorSwatch struct {
	value uint32
	r     rect
}

type dataMapLabel struct {
	text string
	x    int
	y    int
}

type lineSegment struct {
	start int
	end   int
	text  string
	attr  sqdoc.StyleAttr
	face  font.Face
	width int
}

type lineLayout struct {
	block    int
	text     []byte
	segments []lineSegment
	docX     int
	docY     int
	viewX    int
	y        int
	baseline int
	height   int
	ascent   int
	width    int
}

type fontKey struct {
	size   int
	bold   bool
	italic bool
}

type fontBank struct {
	regular    *opentype.Font
	bold       *opentype.Font
	italic     *opentype.Font
	boldItalic *opentype.Font
	cache      map[fontKey]font.Face
}

func newFontBank() fontBank {
	bank := fontBank{cache: map[fontKey]font.Face{}}
	reg, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return bank
	}
	bol, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return bank
	}
	ita, err := opentype.Parse(goitalic.TTF)
	if err != nil {
		return bank
	}
	bit, err := opentype.Parse(gobolditalic.TTF)
	if err != nil {
		return bank
	}
	bank.regular = reg
	bank.bold = bol
	bank.italic = ita
	bank.boldItalic = bit
	return bank
}

type App struct {
	theme ui.Theme
	state *editor.State

	frameBuffer *render.FrameBuffer
	canvas      *ebiten.Image
	docLayer    *ebiten.Image

	fonts fontBank

	uiScales   []float32
	uiScaleIdx int
	filePath   string
	status     string
	frameTick  uint64

	showHelp bool

	undoHistory []snapshot
	redoHistory []snapshot
	maxHistory  int

	topActions      []actionButton
	toolbarActions  []actionButton
	colorSwatches   []colorSwatch
	colorPalette    []uint32
	colorPopupRect  rect
	contentRect     rect
	dataMapRect     rect
	lineLayouts     []lineLayout
	dataMapLabels   []dataMapLabel
	showColorPicker bool
	showDataMap     bool

	fontInputRect   rect
	fontInputActive bool
	fontInputBuffer string

	showEncryption        bool
	encryptionPanel       rect
	encryptionCloseRect   rect
	encryptionEncRect     rect
	encryptionCompRect    rect
	encryptionPassRect    rect
	encryptionInputActive bool
	encryptionEnabled     bool
	compressionEnabled    bool
	encryptionPassword    string

	scrollX float64
	scrollY float64
	maxX    float64
	maxY    float64

	dragSelecting bool
}

func New() *App {
	doc := sqdoc.NewDocument("", "Untitled")
	state := editor.NewState(doc)
	_ = state.UpdateCurrentText("")
	return &App{
		theme:              ui.DefaultTheme(),
		state:              state,
		fonts:              newFontBank(),
		uiScales:           []float32{1.0, 1.25, 1.5, 2.0},
		filePath:           "",
		status:             "Untitled document",
		maxHistory:         200,
		undoHistory:        make([]snapshot, 0, 64),
		redoHistory:        make([]snapshot, 0, 64),
		topActions:         make([]actionButton, 0, 16),
		toolbarActions:     make([]actionButton, 0, 12),
		colorSwatches:      make([]colorSwatch, 0, 16),
		lineLayouts:        make([]lineLayout, 0, 128),
		dataMapLabels:      make([]dataMapLabel, 0, 64),
		colorPalette:       []uint32{0x202020FF, 0x0057B8FF, 0xA31515FF, 0x117A37FF, 0x7A2DB8FF, 0xE67E22FF, 0x8E44ADFF, 0x2C3E50FF, 0xB71C1CFF, 0x00695CFF, 0x455A64FF, 0x000000FF},
		compressionEnabled: true,
	}
}

func (a *App) Run() error {
	ebiten.SetWindowTitle("SIDE - SQDoc Editor")
	ebiten.SetWindowSize(1280, 800)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(900, 560, -1, -1)
	ebiten.MaximizeWindow()
	if err := ebiten.RunGame(a); err != nil {
		return fmt.Errorf("run game loop: %w", err)
	}
	return nil
}

func (a *App) Update() error {
	a.frameTick++
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if a.fontInputActive {
			a.fontInputActive = false
			a.fontInputBuffer = ""
			return nil
		}
		if a.encryptionInputActive {
			a.encryptionInputActive = false
			return nil
		}
		if a.showEncryption {
			a.showEncryption = false
			return nil
		}
		if a.showColorPicker {
			a.showColorPicker = false
			return nil
		}
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		a.showHelp = !a.showHelp
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyP) {
		a.showDataMap = !a.showDataMap
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyE) {
		a.showEncryption = !a.showEncryption
		a.encryptionInputActive = a.showEncryption && a.encryptionEnabled
	}

	if a.handleOverlayTextInput(ctrl) {
		a.clampScroll()
		return nil
	}

	wheelX, wheelY := ebiten.Wheel()
	if shift && wheelY != 0 {
		a.scrollX -= wheelY * 48
	} else if wheelY != 0 {
		a.scrollY -= wheelY * 42
	}
	if wheelX != 0 {
		a.scrollX -= wheelX * 48
	}
	a.clampScroll()

	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		a.scrollY += float64(a.contentRect.h) * 0.8
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		a.scrollY -= float64(a.contentRect.h) * 0.8
	}
	a.clampScroll()

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		if id, ok := a.actionAt(x, y); ok {
			a.invokeAction(id)
			return nil
		}
		if a.showEncryption {
			if a.handleEncryptionClick(x, y) {
				return nil
			}
			return nil
		}
		if handled := a.handleToolbarClick(x, y); handled {
			return nil
		}
		if a.showColorPicker && !a.colorPopupRect.contains(x, y) {
			a.showColorPicker = false
		}
		if a.contentRect.contains(x, y) {
			block, bytePos := a.hitTestPosition(x, y)
			if shift {
				a.state.EnsureSelectionAnchor()
			} else {
				a.state.ClearSelection()
				a.state.EnsureSelectionAnchor()
			}
			a.state.SetCaret(block, bytePos)
			a.state.UpdateSelectionFromCaret()
			a.dragSelecting = true
		} else {
			a.state.ClearSelection()
		}
	}
	if a.dragSelecting && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		block, bytePos := a.hitTestPosition(x, y)
		a.state.SetCaret(block, bytePos)
		a.state.UpdateSelectionFromCaret()
		a.ensureCaretVisible()
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		a.dragSelecting = false
	}

	didSnapshot := false
	recordMutation := func() {
		if didSnapshot {
			return
		}
		a.pushUndoSnapshot()
		didSnapshot = true
	}

	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		a.undo()
		return nil
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyY) {
		a.redo()
		return nil
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyN) {
		a.invokeAction("new")
		return nil
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyO) {
		a.invokeAction("open")
		return nil
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if shift {
			a.invokeAction("save_as")
		} else {
			a.invokeAction("save")
		}
		return nil
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		a.state.SelectAll()
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyC) && !shift {
		if a.state.HasSelection() {
			if err := clipboard.WriteAll(a.state.SelectedText()); err != nil {
				a.status = "Copy failed: " + err.Error()
			}
		}
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyX) {
		if a.state.HasSelection() {
			recordMutation()
			selected := a.state.SelectedText()
			if err := clipboard.WriteAll(selected); err != nil {
				a.status = "Cut failed: " + err.Error()
			} else {
				a.state.DeleteSelection()
			}
		}
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		paste, err := clipboard.ReadAll()
		if err != nil {
			a.status = "Paste failed: " + err.Error()
		} else if paste != "" {
			recordMutation()
			if err := a.state.InsertTextAtCaret(paste); err != nil {
				a.status = "Paste failed: " + err.Error()
			}
		}
	}
	if ctrl && (inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd)) {
		a.bumpUIScale(1)
		a.status = fmt.Sprintf("UI scale %.0f%%", a.uiScales[a.uiScaleIdx]*100)
	}
	if ctrl && (inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract)) {
		a.bumpUIScale(-1)
		a.status = fmt.Sprintf("UI scale %.0f%%", a.uiScales[a.uiScaleIdx]*100)
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyB) {
		recordMutation()
		a.state.ToggleBold()
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyI) {
		recordMutation()
		a.state.ToggleItalic()
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyU) {
		recordMutation()
		a.state.ToggleUnderline()
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		recordMutation()
		a.state.IncreaseFontSize()
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		recordMutation()
		a.state.DecreaseFontSize()
	}
	if ctrl && shift && inpututil.IsKeyJustPressed(ebiten.KeyC) {
		recordMutation()
		a.state.CycleColor()
	}

	moveWithSelection := func(move func()) {
		if shift {
			a.state.EnsureSelectionAnchor()
		} else {
			a.state.ClearSelection()
		}
		move()
		if shift {
			a.state.UpdateSelectionFromCaret()
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		if ctrl {
			moveWithSelection(func() {
				a.state.MoveBlock(-1)
				a.state.MoveCaretToLineStart()
			})
		} else if alt {
			a.scrollY -= float64(a.contentRect.h) * 0.8
		} else {
			moveWithSelection(func() { a.state.MoveBlock(-1) })
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		if ctrl {
			moveWithSelection(func() {
				a.state.MoveBlock(1)
				a.state.MoveCaretToLineStart()
			})
		} else if alt {
			a.scrollY += float64(a.contentRect.h) * 0.8
		} else {
			moveWithSelection(func() { a.state.MoveBlock(1) })
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
		if ctrl {
			moveWithSelection(a.state.MoveCaretWordLeft)
		} else if alt {
			moveWithSelection(a.state.MoveCaretToLineStart)
		} else {
			moveWithSelection(a.state.MoveCaretLeft)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
		if ctrl {
			moveWithSelection(a.state.MoveCaretWordRight)
		} else if alt {
			moveWithSelection(a.state.MoveCaretToLineEnd)
		} else {
			moveWithSelection(a.state.MoveCaretRight)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		if ctrl {
			moveWithSelection(func() { a.state.SetCaret(0, 0) })
		} else {
			moveWithSelection(a.state.MoveCaretToLineStart)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		if ctrl {
			moveWithSelection(func() {
				last := a.state.BlockCount() - 1
				if last >= 0 {
					a.state.SetCaret(last, len(a.state.AllBlockTexts()[last]))
				}
			})
		} else {
			moveWithSelection(a.state.MoveCaretToLineEnd)
		}
	}

	if ctrl || a.showEncryption {
		a.clampScroll()
		a.ensureCaretVisible()
		return nil
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		recordMutation()
		if err := a.state.InsertTextAtCaret("\n"); err != nil {
			a.status = "Insert newline failed: " + err.Error()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		recordMutation()
		a.state.Backspace()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		recordMutation()
		a.state.DeleteForward()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		recordMutation()
		_ = a.state.InsertTextAtCaret("    ")
	}

	for _, r := range ebiten.AppendInputChars(nil) {
		if r < 0x20 || !utf8.ValidRune(r) {
			continue
		}
		recordMutation()
		_ = a.state.InsertTextAtCaret(string(r))
	}

	a.clampScroll()
	a.ensureCaretVisible()
	return nil
}

func (a *App) handleOverlayTextInput(ctrl bool) bool {
	if a.fontInputActive {
		consumed := false
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			if len(a.fontInputBuffer) > 0 {
				a.fontInputBuffer = a.fontInputBuffer[:len(a.fontInputBuffer)-1]
			}
			consumed = true
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
			a.applyFontInput()
			consumed = true
		}
		for _, r := range ebiten.AppendInputChars(nil) {
			if !unicode.IsDigit(r) {
				continue
			}
			if len(a.fontInputBuffer) >= 3 {
				continue
			}
			a.fontInputBuffer += string(r)
			consumed = true
		}
		return consumed
	}

	if a.encryptionInputActive {
		consumed := false
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			if len(a.encryptionPassword) > 0 {
				_, size := utf8.DecodeLastRuneInString(a.encryptionPassword)
				if size <= 0 {
					size = 1
				}
				a.encryptionPassword = a.encryptionPassword[:len(a.encryptionPassword)-size]
			}
			consumed = true
		}
		if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyV) {
			if clip, err := clipboard.ReadAll(); err == nil && clip != "" {
				a.encryptionPassword += clip
				if len(a.encryptionPassword) > 128 {
					a.encryptionPassword = a.encryptionPassword[:128]
				}
			}
			consumed = true
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
			a.encryptionInputActive = false
			consumed = true
		}
		for _, r := range ebiten.AppendInputChars(nil) {
			if r < 0x20 || r == 0x7F || !utf8.ValidRune(r) {
				continue
			}
			a.encryptionPassword += string(r)
			if len(a.encryptionPassword) > 128 {
				a.encryptionPassword = a.encryptionPassword[:128]
			}
			consumed = true
		}
		return consumed
	}
	return false
}

func (a *App) handleEncryptionClick(x, y int) bool {
	if !a.showEncryption {
		return false
	}
	if !a.encryptionPanel.contains(x, y) {
		a.showEncryption = false
		a.encryptionInputActive = false
		return true
	}
	if a.encryptionCloseRect.contains(x, y) {
		a.showEncryption = false
		a.encryptionInputActive = false
		return true
	}
	if a.encryptionCompRect.contains(x, y) {
		a.compressionEnabled = !a.compressionEnabled
		if a.compressionEnabled {
			a.status = "Compression enabled"
		} else {
			a.status = "Compression disabled"
		}
		return true
	}
	if a.encryptionEncRect.contains(x, y) {
		a.encryptionEnabled = !a.encryptionEnabled
		if !a.encryptionEnabled {
			a.encryptionInputActive = false
		}
		if a.encryptionEnabled {
			a.status = "AES-256 encryption enabled"
		} else {
			a.status = "AES-256 encryption disabled"
		}
		return true
	}
	if a.encryptionPassRect.contains(x, y) {
		if a.encryptionEnabled {
			a.encryptionInputActive = true
			a.status = "Encryption password input active"
		} else {
			a.status = "Enable AES-256 first"
		}
		return true
	}
	a.encryptionInputActive = false
	return true
}

func (a *App) handleToolbarClick(x, y int) bool {
	for _, sw := range a.colorSwatches {
		if sw.r.contains(x, y) {
			a.pushUndoSnapshot()
			a.state.SetColor(sw.value)
			a.showColorPicker = false
			a.status = "Applied text color"
			return true
		}
	}
	for _, btn := range a.toolbarActions {
		if btn.r.contains(x, y) {
			a.invokeAction(btn.id)
			return true
		}
	}
	if a.showColorPicker && !a.colorPopupRect.contains(x, y) {
		a.showColorPicker = false
	}
	if a.fontInputActive && !a.fontInputRect.contains(x, y) {
		a.applyFontInput()
		return true
	}
	return false
}

func (a *App) applyFontInput() {
	trimmed := strings.TrimSpace(a.fontInputBuffer)
	a.fontInputActive = false
	a.fontInputBuffer = ""
	if trimmed == "" {
		return
	}
	sz, err := strconv.Atoi(trimmed)
	if err != nil {
		a.status = "Invalid font size"
		return
	}
	if sz < 8 {
		sz = 8
	}
	if sz > 96 {
		sz = 96
	}
	a.pushUndoSnapshot()
	a.state.SetFontSize(uint16(sz))
	a.status = fmt.Sprintf("Font size set to %dpt", sz)
}

func (a *App) actionAt(x, y int) (string, bool) {
	for _, btn := range a.topActions {
		if btn.r.contains(x, y) {
			return btn.id, true
		}
	}
	return "", false
}

func (a *App) invokeAction(id string) {
	switch id {
	case "new":
		a.pushUndoSnapshot()
		a.state = editor.NewState(sqdoc.NewDocument("", "Untitled"))
		a.filePath = ""
		a.status = "New document"
		a.scrollX, a.scrollY = 0, 0
		a.showColorPicker = false
	case "open":
		if err := a.openDocumentDialog(); err != nil {
			a.status = "Open failed: " + err.Error()
		}
	case "save":
		if err := a.saveDocument(false); err != nil {
			a.status = "Save failed: " + err.Error()
		}
	case "save_as":
		if err := a.saveDocument(true); err != nil {
			a.status = "Save As failed: " + err.Error()
		}
	case "undo":
		a.undo()
	case "redo":
		a.redo()
	case "scale_up":
		a.bumpUIScale(1)
		a.status = fmt.Sprintf("UI scale %.0f%%", a.uiScales[a.uiScaleIdx]*100)
	case "scale_down":
		a.bumpUIScale(-1)
		a.status = fmt.Sprintf("UI scale %.0f%%", a.uiScales[a.uiScaleIdx]*100)
	case "help":
		a.showHelp = !a.showHelp
	case "data_map":
		a.showDataMap = !a.showDataMap
	case "encryption":
		a.showEncryption = !a.showEncryption
		a.encryptionInputActive = a.showEncryption && a.encryptionEnabled
	case "bold":
		a.pushUndoSnapshot()
		a.state.ToggleBold()
		if a.state.CurrentStyleAttr().Bold {
			a.status = "Bold on"
		} else {
			a.status = "Bold off"
		}
	case "italic":
		a.pushUndoSnapshot()
		a.state.ToggleItalic()
		if a.state.CurrentStyleAttr().Italic {
			a.status = "Italic on"
		} else {
			a.status = "Italic off"
		}
	case "underline":
		a.pushUndoSnapshot()
		a.state.ToggleUnderline()
		if a.state.CurrentStyleAttr().Underline {
			a.status = "Underline on"
		} else {
			a.status = "Underline off"
		}
	case "font_down":
		a.pushUndoSnapshot()
		a.state.DecreaseFontSize()
		a.status = fmt.Sprintf("Font size %dpt", a.state.CurrentStyleAttr().FontSizePt)
	case "font_up":
		a.pushUndoSnapshot()
		a.state.IncreaseFontSize()
		a.status = fmt.Sprintf("Font size %dpt", a.state.CurrentStyleAttr().FontSizePt)
	case "font_edit":
		a.fontInputActive = true
		a.fontInputBuffer = fmt.Sprintf("%d", a.state.CurrentStyleAttr().FontSizePt)
	case "color_toggle":
		a.showColorPicker = !a.showColorPicker
	}
}

func (a *App) Draw(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	if a.frameBuffer == nil || a.frameBuffer.W != w || a.frameBuffer.H != h {
		a.frameBuffer = render.NewFrameBuffer(w, h)
		a.canvas = ebiten.NewImage(w, h)
	}

	layout := ui.DrawShell(a.frameBuffer, a.state, a.theme, a.uiScales[a.uiScaleIdx])
	menuFace := a.uiFace(11, false)
	toolbarFace := a.uiFace(11, false)
	statusFace := a.uiFace(10, false)
	panelFace := a.uiFace(9, false)

	a.layoutTopActions(menuFace, layout)
	a.layoutToolbarControls(toolbarFace, layout)
	a.layoutContentRects(layout)

	a.drawDocumentChrome(layout)
	a.layoutDocumentLines()
	a.drawDocumentSelectionAndCaret()
	a.drawScrollbars()
	a.drawDataMapPanel()
	a.drawEncryptionPanel(w, h)

	a.canvas.WritePixels(a.frameBuffer.Pixels)
	screen.DrawImage(a.canvas, nil)

	a.drawTopActionLabels(screen, menuFace)
	a.drawToolbarLabels(screen, toolbarFace)
	a.drawDocumentText(screen)
	a.drawDataMapLabels(screen, panelFace)
	a.drawEncryptionLabels(screen, toolbarFace)

	name := a.filePath
	if name == "" {
		name = "Untitled"
	}
	scrollXPct := 0.0
	scrollYPct := 0.0
	if a.maxX > 0 {
		scrollXPct = (a.scrollX / a.maxX) * 100
	}
	if a.maxY > 0 {
		scrollYPct = (a.scrollY / a.maxY) * 100
	}
	attr := a.state.CurrentStyleAttr()
	statusLeft := fmt.Sprintf("Block %d/%d | Caret %d | Font %dpt", a.state.CurrentBlock+1, a.state.BlockCount(), a.state.CaretByte, attr.FontSizePt)
	statusRight := fmt.Sprintf("%s | Scroll X %.0f%% Y %.0f%% | %s", name, scrollXPct, scrollYPct, a.status)
	text.Draw(screen, statusLeft, statusFace, 12, h-10, color.RGBA{R: 42, G: 56, B: 80, A: 255})
	text.Draw(screen, statusRight, statusFace, 320, h-10, color.RGBA{R: 42, G: 56, B: 80, A: 255})

	if a.showHelp {
		a.drawHelpOverlay(screen, toolbarFace)
	}
}

func (a *App) layoutContentRects(layout ui.Layout) {
	textBox := rect{x: layout.ContentX + 10, y: layout.ContentY + 30, w: layout.ContentW - 20, h: layout.ContentH - 34}
	if textBox.w < 280 {
		textBox.w = 280
	}
	if textBox.h < 160 {
		textBox.h = 160
	}
	a.dataMapRect = rect{}
	if a.showDataMap {
		panelW := int(300 * a.uiScales[a.uiScaleIdx])
		if panelW < 260 {
			panelW = 260
		}
		if panelW > textBox.w/2 {
			panelW = textBox.w / 2
		}
		a.dataMapRect = rect{x: textBox.x + textBox.w - panelW, y: textBox.y, w: panelW, h: textBox.h}
		textBox.w -= panelW + 12
	}
	if textBox.w < 220 {
		textBox.w = 220
	}
	if textBox.h < 140 {
		textBox.h = 140
	}
	a.contentRect = textBox
}

func (a *App) drawDocumentChrome(layout ui.Layout) {
	a.frameBuffer.FillRect(layout.ContentX+4, layout.ContentY+4, layout.ContentW-8, layout.ContentH-8, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	a.frameBuffer.StrokeRect(layout.ContentX+4, layout.ContentY+4, layout.ContentW-8, layout.ContentH-8, 1, color.RGBA{R: 187, G: 196, B: 210, A: 255})
	a.frameBuffer.FillRect(layout.ContentX+4, layout.ContentY+4, layout.ContentW-8, 22, color.RGBA{R: 245, G: 248, B: 252, A: 255})
	a.frameBuffer.StrokeRect(layout.ContentX+4, layout.ContentY+4, layout.ContentW-8, 22, 1, color.RGBA{R: 207, G: 214, B: 224, A: 255})

	a.frameBuffer.FillRect(a.contentRect.x, a.contentRect.y, a.contentRect.w, a.contentRect.h, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	a.frameBuffer.StrokeRect(a.contentRect.x, a.contentRect.y, a.contentRect.w, a.contentRect.h, 1, color.RGBA{R: 187, G: 196, B: 210, A: 255})
}

func (a *App) drawTopActionLabels(screen *ebiten.Image, face font.Face) {
	textColor := color.RGBA{R: 244, G: 248, B: 255, A: 255}
	for _, btn := range a.topActions {
		tw := a.measureString(face, btn.label)
		ascent := face.Metrics().Ascent.Round()
		descent := face.Metrics().Descent.Round()
		textHeight := ascent + descent
		x := btn.r.x + (btn.r.w-tw)/2
		baseline := btn.r.y + (btn.r.h+textHeight)/2 - descent
		text.Draw(screen, btn.label, face, x, baseline, textColor)
	}
}

func (a *App) drawToolbarLabels(screen *ebiten.Image, face font.Face) {
	for _, btn := range a.toolbarActions {
		labelColor := color.RGBA{R: 44, G: 58, B: 82, A: 255}
		if btn.active {
			labelColor = color.RGBA{R: 19, G: 62, B: 122, A: 255}
		}
		// choose label text
		label := btn.label
		if btn.id == "font_edit" {
			label = fmt.Sprintf("%d", a.state.CurrentStyleAttr().FontSizePt)
			if a.fontInputActive {
				label = a.fontInputBuffer
			}
		}
		tw := a.measureString(face, label)
		ascent := face.Metrics().Ascent.Round()
		descent := face.Metrics().Descent.Round()
		textHeight := ascent + descent
		x := btn.r.x + (btn.r.w-tw)/2
		baseline := btn.r.y + (btn.r.h+textHeight)/2 - descent
		text.Draw(screen, label, face, x, baseline, labelColor)
	}

	if a.showColorPicker {
		captionFace := a.uiFace(9, false)
		text.Draw(screen, "Color", captionFace, a.colorPopupRect.x+8, a.colorPopupRect.y+14, color.RGBA{R: 44, G: 58, B: 82, A: 255})
	}
}

func (a *App) drawDataMapLabels(screen *ebiten.Image, face font.Face) {
	if !a.showDataMap || a.dataMapRect.w <= 0 || a.dataMapRect.h <= 0 {
		return
	}
	for _, row := range a.dataMapLabels {
		text.Draw(screen, row.text, face, row.x, row.y, color.RGBA{R: 47, G: 60, B: 78, A: 255})
	}
}

func (a *App) drawEncryptionLabels(screen *ebiten.Image, face font.Face) {
	if !a.showEncryption {
		return
	}
	titleFace := a.uiFace(12, true)
	labelFace := a.uiFace(10, false)
	text.Draw(screen, "Encryption View", titleFace, a.encryptionPanel.x+16, a.encryptionPanel.y+24, color.RGBA{R: 24, G: 38, B: 56, A: 255})
	text.Draw(screen, "Close", face, a.encryptionCloseRect.x+18, a.encryptionCloseRect.y+a.encryptionCloseRect.h-8, color.RGBA{R: 42, G: 58, B: 82, A: 255})
	text.Draw(screen, "Compression (zlib)", labelFace, a.encryptionCompRect.x+28, a.encryptionCompRect.y+14, color.RGBA{R: 42, G: 58, B: 82, A: 255})
	text.Draw(screen, "AES-256 password protection", labelFace, a.encryptionEncRect.x+28, a.encryptionEncRect.y+14, color.RGBA{R: 42, G: 58, B: 82, A: 255})
	text.Draw(screen, "Password", labelFace, a.encryptionPassRect.x, a.encryptionPassRect.y-6, color.RGBA{R: 52, G: 66, B: 92, A: 255})

	masked := ""
	if a.encryptionPassword != "" {
		masked = strings.Repeat("*", utf8.RuneCountInString(a.encryptionPassword))
	}
	text.Draw(screen, masked, labelFace, a.encryptionPassRect.x+8, a.encryptionPassRect.y+22, color.RGBA{R: 42, G: 56, B: 80, A: 255})
	if a.encryptionInputActive && (a.frameTick/30)%2 == 0 {
		caretX := a.encryptionPassRect.x + 8 + a.measureString(labelFace, masked)
		ebitenutil.DrawLine(screen, float64(caretX), float64(a.encryptionPassRect.y+7), float64(caretX), float64(a.encryptionPassRect.y+a.encryptionPassRect.h-7), color.RGBA{R: 21, G: 84, B: 164, A: 255})
	}

	hint := "Save/Open use these settings. For encrypted files, set password then open again."
	text.Draw(screen, hint, labelFace, a.encryptionPanel.x+16, a.encryptionPanel.y+a.encryptionPanel.h-12, color.RGBA{R: 74, G: 88, B: 112, A: 255})
}

func (a *App) drawEncryptionPanel(w, h int) {
	if !a.showEncryption {
		return
	}
	panelW := int(520 * a.uiScales[a.uiScaleIdx])
	panelH := int(240 * a.uiScales[a.uiScaleIdx])
	if panelW > w-40 {
		panelW = w - 40
	}
	if panelH > h-40 {
		panelH = h - 40
	}
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	a.encryptionPanel = rect{x: px, y: py, w: panelW, h: panelH}
	a.encryptionCloseRect = rect{x: px + panelW - 88, y: py + 10, w: 72, h: 26}
	a.encryptionCompRect = rect{x: px + 20, y: py + 58, w: 18, h: 18}
	a.encryptionEncRect = rect{x: px + 20, y: py + 92, w: 18, h: 18}
	a.encryptionPassRect = rect{x: px + 20, y: py + 136, w: panelW - 40, h: 30}

	a.frameBuffer.FillRect(px, py, panelW, panelH, color.RGBA{R: 248, G: 250, B: 253, A: 255})
	a.frameBuffer.StrokeRect(px, py, panelW, panelH, 1, color.RGBA{R: 160, G: 176, B: 198, A: 255})

	a.frameBuffer.FillRect(a.encryptionCloseRect.x, a.encryptionCloseRect.y, a.encryptionCloseRect.w, a.encryptionCloseRect.h, color.RGBA{R: 237, G: 242, B: 248, A: 255})
	a.frameBuffer.StrokeRect(a.encryptionCloseRect.x, a.encryptionCloseRect.y, a.encryptionCloseRect.w, a.encryptionCloseRect.h, 1, color.RGBA{R: 172, G: 184, B: 202, A: 255})

	a.drawCheckbox(a.encryptionCompRect, a.compressionEnabled)
	a.drawCheckbox(a.encryptionEncRect, a.encryptionEnabled)

	passBg := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if a.encryptionInputActive {
		passBg = color.RGBA{R: 244, G: 249, B: 255, A: 255}
	}
	a.frameBuffer.FillRect(a.encryptionPassRect.x, a.encryptionPassRect.y, a.encryptionPassRect.w, a.encryptionPassRect.h, passBg)
	border := color.RGBA{R: 170, G: 184, B: 202, A: 255}
	if a.encryptionInputActive {
		border = color.RGBA{R: 77, G: 134, B: 205, A: 255}
	}
	a.frameBuffer.StrokeRect(a.encryptionPassRect.x, a.encryptionPassRect.y, a.encryptionPassRect.w, a.encryptionPassRect.h, 1, border)
}

func (a *App) drawCheckbox(r rect, checked bool) {
	a.frameBuffer.FillRect(r.x, r.y, r.w, r.h, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	a.frameBuffer.StrokeRect(r.x, r.y, r.w, r.h, 1, color.RGBA{R: 130, G: 148, B: 176, A: 255})
	if checked {
		a.frameBuffer.FillRect(r.x+4, r.y+4, r.w-8, r.h-8, color.RGBA{R: 46, G: 102, B: 182, A: 255})
	}
}

func (a *App) drawDataMapPanel() {
	a.dataMapLabels = a.dataMapLabels[:0]
	if !a.showDataMap || a.dataMapRect.w <= 0 || a.dataMapRect.h <= 0 {
		return
	}
	r := a.dataMapRect
	a.frameBuffer.FillRect(r.x, r.y, r.w, r.h, color.RGBA{R: 247, G: 250, B: 254, A: 255})
	a.frameBuffer.StrokeRect(r.x, r.y, r.w, r.h, 1, color.RGBA{R: 188, G: 198, B: 214, A: 255})
	a.frameBuffer.FillRect(r.x, r.y, r.w, 26, color.RGBA{R: 235, G: 241, B: 249, A: 255})

	info, err := sqdoc.InspectLayout(a.state.Doc)
	if err != nil {
		a.dataMapLabels = append(a.dataMapLabels, dataMapLabel{text: "Data map unavailable: " + err.Error(), x: r.x + 10, y: r.y + 44})
		return
	}

	a.dataMapLabels = append(a.dataMapLabels, dataMapLabel{text: "Data Map", x: r.x + 10, y: r.y + 17})
	total := float64(info.FileSize)
	if total <= 0 {
		total = 1
	}
	barX := r.x + 12
	barW := r.w - 24
	y := r.y + 38
	for i, seg := range info.Segments {
		if y+26 > r.y+r.h-10 {
			break
		}
		segW := int(float64(barW) * float64(seg.Length) / total)
		if segW < 8 {
			segW = 8
		}
		if segW > barW {
			segW = barW
		}
		segColor := colorForSegment(seg.Kind, seg.Name, i)
		a.frameBuffer.FillRect(barX, y, segW, 10, segColor)
		a.frameBuffer.StrokeRect(barX, y, segW, 10, 1, color.RGBA{R: 110, G: 126, B: 152, A: 255})
		label := fmt.Sprintf("%s (%dB @%d)", seg.Name, seg.Length, seg.Offset)
		a.dataMapLabels = append(a.dataMapLabels, dataMapLabel{text: label, x: barX, y: y + 22})
		y += 30
	}
}

func colorForSegment(kind sqdoc.BlockKind, name string, idx int) color.RGBA {
	if name == "Header" {
		return color.RGBA{R: 95, G: 125, B: 175, A: 255}
	}
	if name == "Index" {
		return color.RGBA{R: 52, G: 120, B: 199, A: 255}
	}
	switch kind {
	case sqdoc.BlockKindStyle:
		return color.RGBA{R: 188, G: 92, B: 66, A: 255}
	case sqdoc.BlockKindText:
		palette := []color.RGBA{
			{R: 81, G: 142, B: 93, A: 255},
			{R: 54, G: 131, B: 128, A: 255},
			{R: 108, G: 120, B: 182, A: 255},
			{R: 180, G: 126, B: 74, A: 255},
		}
		return palette[idx%len(palette)]
	default:
		return color.RGBA{R: 141, G: 150, B: 166, A: 255}
	}
}

func (a *App) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	if outsideWidth < 900 {
		outsideWidth = 900
	}
	if outsideHeight < 560 {
		outsideHeight = 560
	}
	return outsideWidth, outsideHeight
}

func (a *App) layoutTopActions(face font.Face, layout ui.Layout) {
	a.topActions = a.topActions[:0]
	x := 10
	y := 4
	h := layout.MenuH - 8
	if h < 24 {
		h = 24
	}
	buttons := []actionButton{
		{id: "new", label: "New"},
		{id: "open", label: "Open"},
		{id: "save", label: "Save"},
		{id: "save_as", label: "Save As"},
		{id: "undo", label: "Undo"},
		{id: "redo", label: "Redo"},
		{id: "data_map", label: "Data Map", active: a.showDataMap},
		{id: "encryption", label: "Encryption", active: a.showEncryption},
		{id: "scale_down", label: "A-"},
		{id: "scale_up", label: "A+"},
		{id: "help", label: "Help", active: a.showHelp},
	}
	mx, my := ebiten.CursorPosition()
	for _, btn := range buttons {
		tw := a.measureString(face, btn.label)
		pad := 14
		w := tw + pad*2
		if w < 64 {
			w = 64
		}
		r := rect{x: x, y: y, w: w, h: h}
		hover := r.contains(mx, my)
		bg := color.RGBA{R: 46, G: 84, B: 145, A: 255}
		if btn.active {
			bg = color.RGBA{R: 71, G: 116, B: 186, A: 255}
		}
		if hover {
			bg = color.RGBA{R: 58, G: 102, B: 172, A: 255}
		}
		a.frameBuffer.FillRect(r.x, r.y, r.w, r.h, bg)
		a.frameBuffer.StrokeRect(r.x, r.y, r.w, r.h, 1, color.RGBA{R: 27, G: 54, B: 97, A: 255})
		btn.r = r
		a.topActions = append(a.topActions, btn)
		x += w + 8
	}
}

func (a *App) layoutToolbarControls(face font.Face, layout ui.Layout) {
	a.toolbarActions = a.toolbarActions[:0]
	a.colorSwatches = a.colorSwatches[:0]
	a.colorPopupRect = rect{}
	a.fontInputRect = rect{}

	attr := a.state.CurrentStyleAttr()
	x := 14
	y := layout.MenuH + 8
	h := layout.ToolbarH - 16
	if h < 24 {
		h = 24
	}
	mx, my := ebiten.CursorPosition()

	addBtn := func(id, label string, w int, active bool) rect {
		if w <= 0 {
			tw := a.measureString(face, label)
			pad := 10
			w = tw + pad*2
			if w < 48 {
				w = 48
			}
		}
		r := rect{x: x, y: y, w: w, h: h}
		hover := r.contains(mx, my)
		bg := color.RGBA{R: 241, G: 245, B: 251, A: 255}
		if active {
			bg = color.RGBA{R: 215, G: 229, B: 248, A: 255}
		}
		if hover {
			bg = color.RGBA{R: 223, G: 236, B: 252, A: 255}
		}
		a.frameBuffer.FillRect(r.x, r.y, r.w, r.h, bg)
		a.frameBuffer.StrokeRect(r.x, r.y, r.w, r.h, 1, color.RGBA{R: 181, G: 194, B: 214, A: 255})
		a.toolbarActions = append(a.toolbarActions, actionButton{id: id, label: label, r: r, active: active})
		x += w + 6
		return r
	}

	addBtn("bold", "Bold", 58, attr.Bold)
	addBtn("italic", "Italic", 58, attr.Italic)
	addBtn("underline", "Underline", 78, attr.Underline)
	x += 4

	addBtn("font_down", "-", 28, false)
	fontRect := addBtn("font_edit", "", 56, a.fontInputActive)
	a.fontInputRect = fontRect
	addBtn("font_up", "+", 28, false)
	x += 4

	colorRect := addBtn("color_toggle", "Color", 68, false)
	a.frameBuffer.FillRect(colorRect.x+colorRect.w-14, colorRect.y+6, 8, colorRect.h-12, rgbaFromUint32(attr.ColorRGBA))
	a.frameBuffer.StrokeRect(colorRect.x+colorRect.w-14, colorRect.y+6, 8, colorRect.h-12, 1, color.RGBA{R: 88, G: 102, B: 122, A: 255})

	if a.showColorPicker {
		popupW := 184
		popupH := 88
		px := colorRect.x
		py := colorRect.y + colorRect.h + 4
		a.colorPopupRect = rect{x: px, y: py, w: popupW, h: popupH}
		a.frameBuffer.FillRect(px, py, popupW, popupH, color.RGBA{R: 249, G: 251, B: 254, A: 255})
		a.frameBuffer.StrokeRect(px, py, popupW, popupH, 1, color.RGBA{R: 178, G: 191, B: 210, A: 255})
		cols := 6
		sx := px + 8
		sy := py + 20
		size := 22
		gap := 6
		for i, c := range a.colorPalette {
			cx := sx + (i%cols)*(size+gap)
			cy := sy + (i/cols)*(size+gap)
			r := rect{x: cx, y: cy, w: size, h: size}
			a.frameBuffer.FillRect(r.x, r.y, r.w, r.h, rgbaFromUint32(c))
			border := color.RGBA{R: 118, G: 132, B: 152, A: 255}
			if c == attr.ColorRGBA {
				border = color.RGBA{R: 35, G: 90, B: 170, A: 255}
			}
			a.frameBuffer.StrokeRect(r.x, r.y, r.w, r.h, 1, border)
			a.colorSwatches = append(a.colorSwatches, colorSwatch{value: c, r: r})
		}
	}
}

// measureString returns approximate pixel width of a string for given face.
func (a *App) measureString(face font.Face, s string) int {
	if face == nil || s == "" {
		return 0
	}
	// Use font.MeasureString for accurate advance width (fixed.Int26_6).
	adv := font.MeasureString(face, s)
	// Convert from 26.6 fixed to pixels, round to nearest.
	px := (int(adv) + 32) >> 6
	if px < 0 {
		px = 0
	}
	return px
}

// uiFace returns a cached face for the UI, scaling by current UI scale.
func (a *App) uiFace(size int, bold bool) font.Face {
	key := fontKey{size: size, bold: bold, italic: false}
	if f, ok := a.fonts.cache[key]; ok {
		return f
	}
	var base *opentype.Font
	if bold {
		base = a.fonts.bold
	} else {
		base = a.fonts.regular
	}
	if base == nil {
		return basicfont.Face7x13
	}
	opts := &opentype.FaceOptions{Size: float64(size) * float64(a.uiScales[a.uiScaleIdx]), DPI: 72, Hinting: font.HintingFull}
	face, err := opentype.NewFace(base, opts)
	if err != nil {
		return basicfont.Face7x13
	}
	a.fonts.cache[key] = face
	return face
}

func (a *App) layoutDocumentLines() {
	a.lineLayouts = a.lineLayouts[:0]
	if a.state == nil || a.contentRect.w <= 0 || a.contentRect.h <= 0 {
		return
	}
	y := a.contentRect.y + 8
	padX := a.contentRect.x + 8
	for bi := 0; bi < a.state.BlockCount(); bi++ {
		txt := a.state.AllBlockTexts()[bi]
		lines := strings.Split(txt, "\n")
		byteAcc := 0
		for _, l := range lines {
			ll := lineLayout{block: bi, text: []byte(l), viewX: padX - int(math.Round(a.scrollX)), docY: byteAcc}
			// approximate height based on style font size
			attr := a.state.BlockStyleAttr(bi)
			height := int(float64(attr.FontSizePt) * 1.3 * float64(a.uiScales[a.uiScaleIdx]))
			if height < 12 {
				height = 12
			}
			ll.y = y
			ll.height = height
			ll.ascent = int(float64(attr.FontSizePt) * 0.8)
			ll.text = []byte(l)
			a.lineLayouts = append(a.lineLayouts, ll)
			y += height
			// advance byte accumulator (account for newline except after last line)
			byteAcc += len(l) + 1
		}
		// add spacing between blocks
		y += 6
	}
	// compute max scroll extents
	if y > a.contentRect.y+a.contentRect.h {
		a.maxY = float64(y - (a.contentRect.y + a.contentRect.h))
	} else {
		a.maxY = 0
	}
}

func (a *App) drawDocumentText(screen *ebiten.Image) {
	for _, ll := range a.lineLayouts {
		attr := a.state.BlockStyleAttr(ll.block)
		face := a.uiFace(int(attr.FontSizePt), attr.Bold)
		x := ll.viewX
		y := ll.y - int(math.Round(a.scrollY)) + ll.ascent
		text.Draw(screen, string(ll.text), face, x, y, rgbaFromUint32(attr.ColorRGBA))
	}
}

func (a *App) drawDocumentSelectionAndCaret() {
	// minimal caret rendering
	if a.state == nil {
		return
	}
	cb := a.state.CaretByte
	block := a.state.CurrentBlock
	// find y for block
	px := a.contentRect.x + 8 - int(math.Round(a.scrollX))
	py := a.contentRect.y + 8
	for _, ll := range a.lineLayouts {
		if ll.block != block {
			py += ll.height
			continue
		}
		// caret in this line? map caret byte pos into this line using docY
		lineText := string(ll.text)
		start := ll.docY
		end := ll.docY + len(lineText)
		if cb >= start && cb <= end {
			rel := cb - start
			if rel < 0 {
				rel = 0
			}
			if rel > len(lineText) {
				rel = len(lineText)
			}
			attr := a.state.BlockStyleAttr(block)
			face := a.uiFace(int(attr.FontSizePt), attr.Bold)
			sub := lineText[:rel]
			x := px + a.measureString(face, sub)
			y := ll.y - int(math.Round(a.scrollY))
			// draw caret
			for i := 0; i < 2; i++ {
				a.frameBuffer.FillRect(x+i, y+2, 1, ll.height-4, color.RGBA{R: 21, G: 84, B: 164, A: 255})
			}
			break
		}
		py += ll.height
	}
}

func (a *App) drawScrollbars() {
	// simple visual indicators
	if a.contentRect.w <= 0 || a.contentRect.h <= 0 {
		return
	}
	// vertical
	w := 8
	x := a.contentRect.x + a.contentRect.w - w - 2
	y := a.contentRect.y + 2
	h := a.contentRect.h - 4
	a.frameBuffer.FillRect(x, y, w, h, color.RGBA{R: 245, G: 247, B: 250, A: 255})
	if a.maxY > 0 {
		vh := int(float64(h) * float64(a.contentRect.h) / float64(float64(a.contentRect.h)+a.maxY))
		if vh < 12 {
			vh = 12
		}
		pos := int((a.scrollY / a.maxY) * float64(h-vh))
		if pos < 0 {
			pos = 0
		}
		a.frameBuffer.FillRect(x+1, y+pos+1, w-2, vh-2, color.RGBA{R: 200, G: 210, B: 224, A: 255})
	}
}

func (a *App) hitTestPosition(x, y int) (int, int) {
	// convert screen coords to block and byte offset within that block
	if a.contentRect.contains(x, y) == false {
		return 0, 0
	}
	relY := y - a.contentRect.y + int(math.Round(a.scrollY)) - 8
	acc := 0
	for _, ll := range a.lineLayouts {
		if relY >= acc && relY < acc+ll.height {
			// estimate byte offset by measuring runes
			attr := a.state.BlockStyleAttr(ll.block)
			face := a.uiFace(int(attr.FontSizePt), attr.Bold)
			t := string(ll.text)
			px := ll.viewX
			// iterate runes
			cur := 0
			for i, r := range t {
				// i is byte index within string
				w := a.measureString(face, string(r))
				if px+cur+w/2 >= x {
					return ll.block, ll.docY + i
				}
				cur += w
			}
			return ll.block, ll.docY + len(t)
		}
		acc += ll.height
	}
	// default to end
	last := a.state.BlockCount() - 1
	if last < 0 {
		return 0, 0
	}
	return last, len(a.state.AllBlockTexts()[last])
}

func (a *App) clampScroll() {
	if a.scrollX < 0 {
		a.scrollX = 0
	}
	if a.scrollY < 0 {
		a.scrollY = 0
	}
}

func (a *App) ensureCaretVisible() {
	if a.state == nil {
		return
	}
	// find y position of caret
	block := a.state.CurrentBlock
	bytePos := a.state.CaretByte
	acc := 0
	for _, ll := range a.lineLayouts {
		if ll.block != block {
			acc += ll.height
			continue
		}
		// caret within this line? approximate
		if bytePos <= len(ll.text) {
			y := ll.y - int(math.Round(a.scrollY))
			if y < a.contentRect.y+8 {
				a.scrollY -= float64((a.contentRect.y + 8) - y)
			}
			if y+ll.height > a.contentRect.y+a.contentRect.h-8 {
				a.scrollY += float64(y + ll.height - (a.contentRect.y + a.contentRect.h - 8))
			}
			return
		}
		acc += ll.height
	}
}

func (a *App) pushUndoSnapshot() {
	if a.state == nil || a.state.Doc == nil {
		return
	}
	doc := sqdoc.CloneDocument(a.state.Doc)
	if doc == nil {
		return
	}
	snap := snapshot{doc: doc, currentBlock: a.state.CurrentBlock, caretByte: a.state.CaretByte}
	a.undoHistory = append(a.undoHistory, snap)
	if len(a.undoHistory) > a.maxHistory {
		a.undoHistory = a.undoHistory[1:]
	}
}

func (a *App) undo() {
	if len(a.undoHistory) == 0 {
		return
	}
	last := a.undoHistory[len(a.undoHistory)-1]
	a.undoHistory = a.undoHistory[:len(a.undoHistory)-1]
	if a.state != nil && a.state.Doc != nil {
		// push current into redo
		cur := sqdoc.CloneDocument(a.state.Doc)
		if cur != nil {
			a.redoHistory = append(a.redoHistory, snapshot{doc: cur, currentBlock: a.state.CurrentBlock, caretByte: a.state.CaretByte})
		}
	}
	a.state = editor.NewState(last.doc)
	a.state.CurrentBlock = last.currentBlock
	a.state.CaretByte = last.caretByte
}

func (a *App) redo() {
	if len(a.redoHistory) == 0 {
		return
	}
	last := a.redoHistory[len(a.redoHistory)-1]
	a.redoHistory = a.redoHistory[:len(a.redoHistory)-1]
	if a.state != nil && a.state.Doc != nil {
		cur := sqdoc.CloneDocument(a.state.Doc)
		if cur != nil {
			a.undoHistory = append(a.undoHistory, snapshot{doc: cur, currentBlock: a.state.CurrentBlock, caretByte: a.state.CaretByte})
		}
	}
	a.state = editor.NewState(last.doc)
	a.state.CurrentBlock = last.currentBlock
	a.state.CaretByte = last.caretByte
}

func (a *App) openDocumentDialog() error {
	path, err := dialog.File().Filter("SQDoc files", "sqdoc").Load()
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("no file selected")
	}
	doc, err := sqdoc.LoadWithOptions(path, sqdoc.LoadOptions{Password: a.encryptionPassword})
	if err != nil {
		return err
	}
	a.state = editor.NewState(doc)
	a.filePath = path
	a.status = "Opened " + filepath.Base(path)
	return nil
}

func (a *App) saveDocument(saveAs bool) error {
	path := a.filePath
	if saveAs || path == "" {
		p, err := dialog.File().Filter("SQDoc files", "sqdoc").Save()
		if err != nil {
			return err
		}
		path = p
	}
	if path == "" {
		return errors.New("no file selected")
	}
	if a.state == nil || a.state.Doc == nil {
		return errors.New("no document to save")
	}
	opts := sqdoc.SaveOptions{Compression: a.compressionEnabled, Encryption: sqdoc.EncryptionOptions{Enabled: a.encryptionEnabled, Password: a.encryptionPassword}}
	if err := sqdoc.SaveWithOptions(path, a.state.Doc, opts); err != nil {
		return err
	}
	a.filePath = path
	a.status = "Saved " + filepath.Base(path)
	return nil
}

func (a *App) bumpUIScale(delta int) {
	if len(a.uiScales) == 0 {
		return
	}
	a.uiScaleIdx += delta
	if a.uiScaleIdx < 0 {
		a.uiScaleIdx = 0
	}
	if a.uiScaleIdx >= len(a.uiScales) {
		a.uiScaleIdx = len(a.uiScales) - 1
	}
}

func (a *App) drawHelpOverlay(screen *ebiten.Image, face font.Face) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	panelW := int(float64(w) * 0.6)
	panelH := int(float64(h) * 0.6)
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	a.frameBuffer.FillRect(px, py, panelW, panelH, color.RGBA{R: 250, G: 251, B: 253, A: 255})
	a.frameBuffer.StrokeRect(px, py, panelW, panelH, 1, color.RGBA{R: 170, G: 184, B: 202, A: 255})
	lines := []string{
		"Help - Keyboard Shortcuts:",
		"Ctrl+S: Save | Ctrl+Shift+S: Save As",
		"Ctrl+O: Open | Ctrl+N: New",
		"Ctrl+Z: Undo | Ctrl+Y: Redo",
		"Ctrl+B/I/U: Bold / Italic / Underline",
		"Mouse wheel: vertical scroll | Shift+wheel: horizontal",
		"Click inside document to set caret; drag to select",
	}
	y := py + 28
	for _, l := range lines {
		text.Draw(screen, l, face, px+20, y, color.RGBA{R: 48, G: 60, B: 78, A: 255})
		y += 22
	}
}

func rgbaFromUint32(u uint32) color.RGBA {
	return color.RGBA{R: uint8((u >> 24) & 0xFF), G: uint8((u >> 16) & 0xFF), B: uint8((u >> 8) & 0xFF), A: uint8(u & 0xFF)}
}
