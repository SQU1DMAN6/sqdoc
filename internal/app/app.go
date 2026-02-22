package app

import (
	"bytes"
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
	block     int
	startByte int
	text      []byte
	segments  []lineSegment
	docX      int
	docY      int
	viewX     int
	y         int
	baseline  int
	height    int
	ascent    int
	width     int
}

type fontKey struct {
	size   int
	bold   bool
	italic bool
	scale  int
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

	showHelp  bool
	helpRect  rect
	helpClose rect

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

	showPasswordPrompt    bool
	passwordPromptRect    rect
	passwordInputRect     rect
	passwordSubmitRect    rect
	passwordCancelRect    rect
	passwordPromptPath    string
	passwordPromptInput   string
	passwordPromptError   string
	passwordPromptFocused bool

	scrollX float64
	scrollY float64
	maxX    float64
	maxY    float64

	dragSelecting bool

	screenW int
	screenH int
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
	winW, winH := a.currentViewportSize()
	if a.showEncryption {
		a.layoutEncryptionPanelBounds(winW, winH)
	}
	if a.showPasswordPrompt {
		a.layoutPasswordPromptBounds(winW, winH)
	}
	if a.showHelp {
		a.layoutHelpDialogBounds(winW, winH)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if a.showPasswordPrompt {
			a.closePasswordPrompt()
			return nil
		}
		if a.showHelp {
			a.showHelp = false
			return nil
		}
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
	if a.showHelp {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			x, y := ebiten.CursorPosition()
			if !a.helpRect.contains(x, y) || a.helpClose.contains(x, y) {
				a.showHelp = false
			}
		}
		a.clampScroll()
		return nil
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
		if a.showPasswordPrompt {
			a.handlePasswordPromptClick(x, y)
			return nil
		}
		if a.showEncryption {
			if a.handleEncryptionClick(x, y) {
				return nil
			}
			return nil
		}
		if id, ok := a.actionAt(x, y); ok {
			a.invokeAction(id)
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

	if a.showPasswordPrompt {
		a.clampScroll()
		return nil
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
	if ctrl && shift && inpututil.IsKeyJustPressed(ebiten.KeyH) {
		recordMutation()
		a.state.ToggleHighlight()
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		recordMutation()
		a.state.DeleteWordBackward()
	}
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		recordMutation()
		a.state.DeleteWordForward()
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

	if ctrl || a.showEncryption || a.showPasswordPrompt {
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
	if a.showPasswordPrompt {
		consumed := false
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			if len(a.passwordPromptInput) > 0 {
				_, size := utf8.DecodeLastRuneInString(a.passwordPromptInput)
				if size <= 0 {
					size = 1
				}
				a.passwordPromptInput = a.passwordPromptInput[:len(a.passwordPromptInput)-size]
			}
			consumed = true
		}
		if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyV) {
			if clip, err := clipboard.ReadAll(); err == nil && clip != "" {
				a.passwordPromptInput += clip
				if len(a.passwordPromptInput) > 128 {
					a.passwordPromptInput = a.passwordPromptInput[:128]
				}
			}
			consumed = true
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
			a.submitPasswordPrompt()
			consumed = true
		}
		for _, r := range ebiten.AppendInputChars(nil) {
			if r < 0x20 || r == 0x7F || !utf8.ValidRune(r) {
				continue
			}
			a.passwordPromptInput += string(r)
			if len(a.passwordPromptInput) > 128 {
				a.passwordPromptInput = a.passwordPromptInput[:128]
			}
			consumed = true
		}
		return consumed
	}

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
	// Keep the encryption panel modal: all clicks are consumed while open.
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

func (a *App) handlePasswordPromptClick(x, y int) {
	if !a.showPasswordPrompt {
		return
	}
	if !a.passwordPromptRect.contains(x, y) {
		a.closePasswordPrompt()
		return
	}
	if a.passwordInputRect.contains(x, y) {
		a.passwordPromptFocused = true
		return
	}
	a.passwordPromptFocused = false
	if a.passwordSubmitRect.contains(x, y) {
		a.submitPasswordPrompt()
		return
	}
	if a.passwordCancelRect.contains(x, y) {
		a.closePasswordPrompt()
		return
	}
}

func (a *App) submitPasswordPrompt() {
	path := strings.TrimSpace(a.passwordPromptPath)
	if path == "" {
		a.closePasswordPrompt()
		return
	}
	env, envErr := sqdoc.InspectEnvelope(filepath.Clean(path))
	if envErr != nil {
		a.status = "Open failed: " + envErr.Error()
		a.closePasswordPrompt()
		return
	}
	doc, err := sqdoc.LoadWithOptions(filepath.Clean(path), sqdoc.LoadOptions{Password: a.passwordPromptInput})
	if err != nil {
		if errors.Is(err, sqdoc.ErrPasswordRequired) || errors.Is(err, sqdoc.ErrInvalidPassword) {
			a.passwordPromptError = "Incorrect password. Try again."
			return
		}
		a.status = "Open failed: " + err.Error()
		a.closePasswordPrompt()
		return
	}

	a.state = editor.NewState(doc)
	a.filePath = path
	a.status = "Opened " + filepath.Base(path)
	a.scrollX, a.scrollY = 0, 0
	a.maxX, a.maxY = 0, 0
	a.undoHistory = a.undoHistory[:0]
	a.redoHistory = a.redoHistory[:0]
	a.encryptionPassword = a.passwordPromptInput
	a.applyEnvelopeSettings(env)
	a.closePasswordPrompt()
}

func (a *App) closePasswordPrompt() {
	a.showPasswordPrompt = false
	a.passwordPromptFocused = false
	a.passwordPromptPath = ""
	a.passwordPromptInput = ""
	a.passwordPromptError = ""
}

func (a *App) applyEnvelopeSettings(info sqdoc.EnvelopeInfo) {
	if info.Wrapped {
		a.compressionEnabled = info.Compressed
		a.encryptionEnabled = info.Encrypted
		return
	}
	a.compressionEnabled = false
	a.encryptionEnabled = false
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
	case "highlight":
		a.pushUndoSnapshot()
		a.state.ToggleHighlight()
		if a.state.CurrentStyleAttr().Highlight {
			a.status = "Highlight on"
		} else {
			a.status = "Highlight off"
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
	menuFace := a.uiFace(11, false, false)
	toolbarFace := a.uiFace(11, false, false)
	statusFace := a.uiFace(10, false, false)
	panelFace := a.uiFace(9, false, false)

	a.layoutTopActions(menuFace, layout)
	a.layoutToolbarControls(toolbarFace, layout)
	a.layoutContentRects(layout)

	a.drawDocumentChrome(layout)
	a.layoutDocumentLines()
	a.drawDocumentSelectionAndCaret()
	a.drawScrollbars()
	a.drawDataMapPanel()
	if a.showEncryption {
		a.layoutEncryptionPanelBounds(w, h)
	}
	if a.showPasswordPrompt {
		a.layoutPasswordPromptBounds(w, h)
	}

	a.canvas.WritePixels(a.frameBuffer.Pixels)
	screen.DrawImage(a.canvas, nil)

	a.drawTopActionLabels(screen, menuFace)
	a.drawToolbarLabels(screen, toolbarFace)
	a.drawDocumentText(screen)
	a.drawDataMapLabels(screen, panelFace)

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
	statusLeft := fmt.Sprintf("[ Block %d/%d ] [ Caret %d ] [ Font %dpt ]", a.state.CurrentBlock+1, a.state.BlockCount(), a.state.CaretByte, attr.FontSizePt)
	statusRight := fmt.Sprintf("[ %s ] [ Scroll X %.0f%% Y %.0f%% ] [ %s ]", name, scrollXPct, scrollYPct, a.status)
	text.Draw(screen, statusLeft, statusFace, 12, h-10, color.RGBA{R: 42, G: 56, B: 80, A: 255})
	text.Draw(screen, statusRight, statusFace, 320, h-10, color.RGBA{R: 42, G: 56, B: 80, A: 255})

	a.drawColorPickerOverlay(screen)
	a.drawEncryptionPanel(screen, w, h)
	a.drawEncryptionLabels(screen, toolbarFace)
	a.drawPasswordPrompt(screen, w, h)

	if a.showHelp {
		a.drawHelpOverlay(screen, toolbarFace)
	}
}

func (a *App) layoutContentRects(layout ui.Layout) {
	textBox := rect{x: layout.ContentX + 6, y: layout.ContentY + 24, w: layout.ContentW - 12, h: layout.ContentH - 28}
	if textBox.w < 360 {
		textBox.w = 360
	}
	if textBox.h < 220 {
		textBox.h = 220
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
	if textBox.w < 260 {
		textBox.w = 260
	}
	if textBox.h < 180 {
		textBox.h = 180
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
	titleFace := a.uiFace(12, true, false)
	labelFace := a.uiFace(10, false, false)
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

func (a *App) layoutEncryptionPanelBounds(w, h int) {
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
}

func (a *App) drawEncryptionPanel(screen *ebiten.Image, w, h int) {
	if !a.showEncryption {
		return
	}
	a.layoutEncryptionPanelBounds(w, h)
	px, py, panelW, panelH := a.encryptionPanel.x, a.encryptionPanel.y, a.encryptionPanel.w, a.encryptionPanel.h

	a.drawFilledRectOnScreen(screen, px, py, panelW, panelH, color.RGBA{R: 248, G: 250, B: 253, A: 255})
	ebitenutil.DrawLine(screen, float64(px), float64(py), float64(px+panelW), float64(py), color.RGBA{R: 160, G: 176, B: 198, A: 255})
	ebitenutil.DrawLine(screen, float64(px), float64(py+panelH), float64(px+panelW), float64(py+panelH), color.RGBA{R: 160, G: 176, B: 198, A: 255})
	ebitenutil.DrawLine(screen, float64(px), float64(py), float64(px), float64(py+panelH), color.RGBA{R: 160, G: 176, B: 198, A: 255})
	ebitenutil.DrawLine(screen, float64(px+panelW), float64(py), float64(px+panelW), float64(py+panelH), color.RGBA{R: 160, G: 176, B: 198, A: 255})

	a.drawFilledRectOnScreen(screen, a.encryptionCloseRect.x, a.encryptionCloseRect.y, a.encryptionCloseRect.w, a.encryptionCloseRect.h, color.RGBA{R: 237, G: 242, B: 248, A: 255})
	ebitenutil.DrawLine(screen, float64(a.encryptionCloseRect.x), float64(a.encryptionCloseRect.y), float64(a.encryptionCloseRect.x+a.encryptionCloseRect.w), float64(a.encryptionCloseRect.y), color.RGBA{R: 172, G: 184, B: 202, A: 255})
	ebitenutil.DrawLine(screen, float64(a.encryptionCloseRect.x), float64(a.encryptionCloseRect.y+a.encryptionCloseRect.h), float64(a.encryptionCloseRect.x+a.encryptionCloseRect.w), float64(a.encryptionCloseRect.y+a.encryptionCloseRect.h), color.RGBA{R: 172, G: 184, B: 202, A: 255})
	ebitenutil.DrawLine(screen, float64(a.encryptionCloseRect.x), float64(a.encryptionCloseRect.y), float64(a.encryptionCloseRect.x), float64(a.encryptionCloseRect.y+a.encryptionCloseRect.h), color.RGBA{R: 172, G: 184, B: 202, A: 255})
	ebitenutil.DrawLine(screen, float64(a.encryptionCloseRect.x+a.encryptionCloseRect.w), float64(a.encryptionCloseRect.y), float64(a.encryptionCloseRect.x+a.encryptionCloseRect.w), float64(a.encryptionCloseRect.y+a.encryptionCloseRect.h), color.RGBA{R: 172, G: 184, B: 202, A: 255})

	a.drawCheckbox(screen, a.encryptionCompRect, a.compressionEnabled)
	a.drawCheckbox(screen, a.encryptionEncRect, a.encryptionEnabled)

	passBg := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if a.encryptionInputActive {
		passBg = color.RGBA{R: 244, G: 249, B: 255, A: 255}
	}
	a.drawFilledRectOnScreen(screen, a.encryptionPassRect.x, a.encryptionPassRect.y, a.encryptionPassRect.w, a.encryptionPassRect.h, passBg)
	border := color.RGBA{R: 170, G: 184, B: 202, A: 255}
	if a.encryptionInputActive {
		border = color.RGBA{R: 77, G: 134, B: 205, A: 255}
	}
	ebitenutil.DrawLine(screen, float64(a.encryptionPassRect.x), float64(a.encryptionPassRect.y), float64(a.encryptionPassRect.x+a.encryptionPassRect.w), float64(a.encryptionPassRect.y), border)
	ebitenutil.DrawLine(screen, float64(a.encryptionPassRect.x), float64(a.encryptionPassRect.y+a.encryptionPassRect.h), float64(a.encryptionPassRect.x+a.encryptionPassRect.w), float64(a.encryptionPassRect.y+a.encryptionPassRect.h), border)
	ebitenutil.DrawLine(screen, float64(a.encryptionPassRect.x), float64(a.encryptionPassRect.y), float64(a.encryptionPassRect.x), float64(a.encryptionPassRect.y+a.encryptionPassRect.h), border)
	ebitenutil.DrawLine(screen, float64(a.encryptionPassRect.x+a.encryptionPassRect.w), float64(a.encryptionPassRect.y), float64(a.encryptionPassRect.x+a.encryptionPassRect.w), float64(a.encryptionPassRect.y+a.encryptionPassRect.h), border)
}

func (a *App) drawCheckbox(screen *ebiten.Image, r rect, checked bool) {
	a.drawFilledRectOnScreen(screen, r.x, r.y, r.w, r.h, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y), float64(r.x+r.w), float64(r.y), color.RGBA{R: 130, G: 148, B: 176, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y+r.h), float64(r.x+r.w), float64(r.y+r.h), color.RGBA{R: 130, G: 148, B: 176, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y), float64(r.x), float64(r.y+r.h), color.RGBA{R: 130, G: 148, B: 176, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x+r.w), float64(r.y), float64(r.x+r.w), float64(r.y+r.h), color.RGBA{R: 130, G: 148, B: 176, A: 255})
	if checked {
		a.drawFilledRectOnScreen(screen, r.x+4, r.y+4, r.w-8, r.h-8, color.RGBA{R: 46, G: 102, B: 182, A: 255})
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
	a.screenW = outsideWidth
	a.screenH = outsideHeight
	return outsideWidth, outsideHeight
}

func (a *App) currentViewportSize() (int, int) {
	if a.screenW > 0 && a.screenH > 0 {
		return a.screenW, a.screenH
	}
	w, h := ebiten.WindowSize()
	if w <= 0 {
		w = 1280
	}
	if h <= 0 {
		h = 800
	}
	return w, h
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
	addBtn("highlight", "Highlight", 82, attr.Highlight)
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
		cols := 6
		sx := px + 8
		sy := py + 20
		size := 22
		gap := 6
		for i, c := range a.colorPalette {
			cx := sx + (i%cols)*(size+gap)
			cy := sy + (i/cols)*(size+gap)
			r := rect{x: cx, y: cy, w: size, h: size}
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
func (a *App) uiFace(size int, bold, italic bool) font.Face {
	scaleKey := int(math.Round(float64(a.uiScales[a.uiScaleIdx] * 1000)))
	key := fontKey{size: size, bold: bold, italic: italic, scale: scaleKey}
	if f, ok := a.fonts.cache[key]; ok {
		return f
	}
	var base *opentype.Font
	switch {
	case bold && italic:
		base = a.fonts.boldItalic
	case bold:
		base = a.fonts.bold
	case italic:
		base = a.fonts.italic
	default:
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

	docY := 4
	lineGap := int(4 * a.uiScales[a.uiScaleIdx])
	if lineGap < 2 {
		lineGap = 2
	}
	blockGap := int(8 * a.uiScales[a.uiScaleIdx])
	if blockGap < 6 {
		blockGap = 6
	}
	maxWidth := 0

	for bi := 0; bi < a.state.BlockCount(); bi++ {
		textBytes := []byte(a.state.AllBlockTexts()[bi])
		runs := a.state.BlockRuns(bi)
		if len(runs) == 0 {
			runs = []sqdoc.StyleRun{{Start: 0, End: uint32(len(textBytes)), Attr: defaultAttr()}}
		}

		lineStart := 0
		for {
			relEnd := bytes.IndexByte(textBytes[lineStart:], '\n')
			lineEnd := len(textBytes)
			hasNL := false
			if relEnd >= 0 {
				lineEnd = lineStart + relEnd
				hasNL = true
			}

			lineBytes := append([]byte(nil), textBytes[lineStart:lineEnd]...)
			lineLen := len(lineBytes)
			segments := make([]lineSegment, 0, len(runs))
			lineWidth := 0
			maxAscent := 0
			maxDescent := 0

			for _, run := range runs {
				rs := int(run.Start)
				re := int(run.End)
				if re <= lineStart || rs >= lineEnd {
					continue
				}
				segStart := max(rs, lineStart) - lineStart
				segEnd := min(re, lineEnd) - lineStart
				if segEnd < segStart {
					continue
				}
				attr := run.Attr
				if attr.FontSizePt == 0 {
					attr.FontSizePt = 14
				}
				if attr.ColorRGBA == 0 {
					attr.ColorRGBA = 0x202020FF
				}
				face := a.uiFace(int(attr.FontSizePt), attr.Bold, attr.Italic)
				segText := ""
				if segStart < segEnd && segEnd <= lineLen {
					segText = string(lineBytes[segStart:segEnd])
				}
				segW := a.measureString(face, segText)
				m := face.Metrics()
				if asc := m.Ascent.Round(); asc > maxAscent {
					maxAscent = asc
				}
				if des := m.Descent.Round(); des > maxDescent {
					maxDescent = des
				}
				segments = append(segments, lineSegment{
					start: segStart,
					end:   segEnd,
					text:  segText,
					attr:  attr,
					face:  face,
					width: segW,
				})
				lineWidth += segW
			}

			if len(segments) == 0 {
				attr := defaultAttr()
				if len(runs) > 0 {
					attr = runs[0].Attr
				}
				face := a.uiFace(int(attr.FontSizePt), attr.Bold, attr.Italic)
				m := face.Metrics()
				maxAscent = m.Ascent.Round()
				maxDescent = m.Descent.Round()
				segments = append(segments, lineSegment{
					start: 0,
					end:   lineLen,
					text:  string(lineBytes),
					attr:  attr,
					face:  face,
					width: a.measureString(face, string(lineBytes)),
				})
				lineWidth = segments[0].width
			}

			height := maxAscent + maxDescent + int(6*a.uiScales[a.uiScaleIdx])
			if height < 18 {
				height = 18
			}
			a.lineLayouts = append(a.lineLayouts, lineLayout{
				block:     bi,
				startByte: lineStart,
				text:      lineBytes,
				segments:  segments,
				docX:      8,
				docY:      docY,
				height:    height,
				ascent:    maxAscent,
				width:     lineWidth,
			})

			if 8+lineWidth > maxWidth {
				maxWidth = 8 + lineWidth
			}
			docY += height + lineGap

			if !hasNL {
				break
			}
			lineStart = lineEnd + 1
		}
		docY += blockGap
	}

	contentW := max(1, a.contentRect.w-12)
	totalHeight := docY + 6
	a.maxY = math.Max(0, float64(totalHeight-a.contentRect.h))
	a.maxX = math.Max(0, float64(maxWidth-contentW))
	a.clampScroll()

	for i := range a.lineLayouts {
		a.lineLayouts[i].y = a.contentRect.y + a.lineLayouts[i].docY - int(a.scrollY)
		a.lineLayouts[i].viewX = a.contentRect.x + a.lineLayouts[i].docX - int(a.scrollX)
		a.lineLayouts[i].baseline = a.lineLayouts[i].y + a.lineLayouts[i].ascent + 1
	}
}

func (a *App) drawDocumentText(screen *ebiten.Image) {
	if a.contentRect.w <= 0 || a.contentRect.h <= 0 {
		return
	}
	if a.docLayer == nil || a.docLayer.Bounds().Dx() != a.contentRect.w || a.docLayer.Bounds().Dy() != a.contentRect.h {
		a.docLayer = ebiten.NewImage(max(1, a.contentRect.w), max(1, a.contentRect.h))
	}
	a.docLayer.Clear()

	highlightColor := color.RGBA{R: 255, G: 244, B: 168, A: 255}

	for _, ll := range a.lineLayouts {
		relY := ll.y - a.contentRect.y
		if relY+ll.height < 0 || relY > a.contentRect.h {
			continue
		}
		x := ll.viewX - a.contentRect.x
		baseline := ll.baseline - a.contentRect.y
		for _, seg := range ll.segments {
			segX := x
			if seg.attr.Highlight && seg.width > 0 {
				top := baseline - seg.face.Metrics().Ascent.Round()
				h := seg.face.Metrics().Ascent.Round() + seg.face.Metrics().Descent.Round()
				if h < 12 {
					h = 12
				}
				a.drawFilledRectOnScreen(a.docLayer, segX, top, seg.width, h, highlightColor)
			}
			if seg.text != "" {
				clr := rgbaFromUint32(seg.attr.ColorRGBA)
				text.Draw(a.docLayer, seg.text, seg.face, segX, baseline, clr)
				if seg.attr.Underline {
					underlineY := float64(baseline + max(1, seg.face.Metrics().Descent.Round()/2))
					ebitenutil.DrawLine(a.docLayer, float64(segX), underlineY, float64(segX+seg.width), underlineY, clr)
				}
			}
			x += seg.width
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(a.contentRect.x), float64(a.contentRect.y))
	screen.DrawImage(a.docLayer, op)
}

func (a *App) drawDocumentSelectionAndCaret() {
	if a.state == nil {
		return
	}

	selColor := color.RGBA{R: 191, G: 214, B: 255, A: 255}
	if start, end, ok := a.state.SelectionRange(); ok {
		for _, ll := range a.lineLayouts {
			if ll.block < start.Block || ll.block > end.Block {
				continue
			}
			lineStart := ll.startByte
			lineEnd := ll.startByte + len(ll.text)
			selStart := lineStart
			selEnd := lineEnd
			if ll.block == start.Block {
				selStart = start.Byte
			}
			if ll.block == end.Block {
				selEnd = end.Byte
			}
			selStart = max(selStart, lineStart)
			selEnd = min(selEnd, lineEnd)
			if selEnd <= selStart {
				continue
			}
			x0 := ll.viewX + a.lineAdvance(ll, selStart-lineStart)
			x1 := ll.viewX + a.lineAdvance(ll, selEnd-lineStart)
			a.fillRectWithinContent(x0, ll.y+1, x1-x0, ll.height-2, selColor)
		}
	}

	if a.state.HasSelection() || (a.frameTick/30)%2 == 0 {
		return
	}
	block := a.state.CurrentBlock
	caret := a.state.CaretByte
	for _, ll := range a.lineLayouts {
		if ll.block != block {
			continue
		}
		lineStart := ll.startByte
		lineEnd := lineStart + len(ll.text)
		if caret < lineStart || caret > lineEnd {
			continue
		}
		rel := caret - lineStart
		x := ll.viewX + a.lineAdvance(ll, rel)
		a.fillRectWithinContent(x, ll.y+2, 1, max(2, ll.height-4), color.RGBA{R: 21, G: 84, B: 164, A: 255})
		break
	}
}

func (a *App) drawScrollbars() {
	if a.contentRect.w <= 0 || a.contentRect.h <= 0 {
		return
	}

	if a.maxY > 0 {
		trackX := a.contentRect.x + a.contentRect.w - 6
		trackY := a.contentRect.y + 2
		trackH := a.contentRect.h - 8
		a.frameBuffer.FillRect(trackX, trackY, 4, trackH, color.RGBA{R: 231, G: 236, B: 244, A: 255})
		thumbH := max(24, int(float64(trackH)*float64(a.contentRect.h)/(float64(a.contentRect.h)+a.maxY)))
		thumbY := trackY
		if a.maxY > 0 {
			thumbY = trackY + int((a.scrollY/a.maxY)*float64(trackH-thumbH))
		}
		a.frameBuffer.FillRect(trackX, thumbY, 4, thumbH, color.RGBA{R: 156, G: 170, B: 190, A: 255})
	}
	if a.maxX > 0 {
		trackX := a.contentRect.x + 2
		trackY := a.contentRect.y + a.contentRect.h - 6
		trackW := a.contentRect.w - 8
		a.frameBuffer.FillRect(trackX, trackY, trackW, 4, color.RGBA{R: 231, G: 236, B: 244, A: 255})
		thumbW := max(24, int(float64(trackW)*float64(a.contentRect.w)/(float64(a.contentRect.w)+a.maxX)))
		thumbX := trackX
		if a.maxX > 0 {
			thumbX = trackX + int((a.scrollX/a.maxX)*float64(trackW-thumbW))
		}
		a.frameBuffer.FillRect(thumbX, trackY, thumbW, 4, color.RGBA{R: 156, G: 170, B: 190, A: 255})
	}
}

// drawContentBorderOverlay draws the content rect borders on screen to cover overflowed text.
func (a *App) drawContentBorderOverlay(screen *ebiten.Image) {
	// stroke color same as frameBuffer strokes used earlier
	c := color.RGBA{R: 187, G: 196, B: 210, A: 255}
	x := a.contentRect.x
	y := a.contentRect.y
	w := a.contentRect.w
	h := a.contentRect.h
	// top
	ebitenutil.DrawLine(screen, float64(x), float64(y), float64(x+w), float64(y), c)
	// bottom
	ebitenutil.DrawLine(screen, float64(x), float64(y+h), float64(x+w), float64(y+h), c)
	// left
	ebitenutil.DrawLine(screen, float64(x), float64(y), float64(x), float64(y+h), c)
	// right
	ebitenutil.DrawLine(screen, float64(x+w), float64(y), float64(x+w), float64(y+h), c)
}

// drawFilledRectOnScreen draws a filled rectangle on the screen by drawing horizontal lines.
func (a *App) drawFilledRectOnScreen(screen *ebiten.Image, x, y, w, h int, c color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		ebitenutil.DrawLine(screen, float64(x), float64(yy), float64(x+w), float64(yy), c)
	}
}

func (a *App) drawColorPickerOverlay(screen *ebiten.Image) {
	if !a.showColorPicker || a.colorPopupRect.w == 0 {
		return
	}
	px := a.colorPopupRect.x
	py := a.colorPopupRect.y
	pw := a.colorPopupRect.w
	ph := a.colorPopupRect.h
	// background
	a.drawFilledRectOnScreen(screen, px, py, pw, ph, color.RGBA{R: 249, G: 251, B: 254, A: 255})
	// border
	ebitenutil.DrawLine(screen, float64(px), float64(py), float64(px+pw), float64(py), color.RGBA{R: 178, G: 191, B: 210, A: 255})
	ebitenutil.DrawLine(screen, float64(px), float64(py+ph), float64(px+pw), float64(py+ph), color.RGBA{R: 178, G: 191, B: 210, A: 255})
	ebitenutil.DrawLine(screen, float64(px), float64(py), float64(px), float64(py+ph), color.RGBA{R: 178, G: 191, B: 210, A: 255})
	ebitenutil.DrawLine(screen, float64(px+pw), float64(py), float64(px+pw), float64(py+ph), color.RGBA{R: 178, G: 191, B: 210, A: 255})
	// caption
	captionFace := a.uiFace(9, false, false)
	text.Draw(screen, "Color", captionFace, px+8, py+14, color.RGBA{R: 44, G: 58, B: 82, A: 255})
	// swatches (re-use positions in a.colorSwatches)
	for _, sw := range a.colorSwatches {
		// draw filled rect on screen
		a.drawFilledRectOnScreen(screen, sw.r.x, sw.r.y, sw.r.w, sw.r.h, rgbaFromUint32(sw.value))
		// border
		ebitenutil.DrawLine(screen, float64(sw.r.x), float64(sw.r.y), float64(sw.r.x+sw.r.w), float64(sw.r.y), color.RGBA{R: 118, G: 132, B: 152, A: 255})
		ebitenutil.DrawLine(screen, float64(sw.r.x), float64(sw.r.y+sw.r.h), float64(sw.r.x+sw.r.w), float64(sw.r.y+sw.r.h), color.RGBA{R: 118, G: 132, B: 152, A: 255})
	}
}

func (a *App) layoutPasswordPromptBounds(w, h int) {
	pw := int(460 * a.uiScales[a.uiScaleIdx])
	ph := int(210 * a.uiScales[a.uiScaleIdx])
	if pw > w-40 {
		pw = w - 40
	}
	if ph > h-40 {
		ph = h - 40
	}
	px := (w - pw) / 2
	py := (h - ph) / 2
	a.passwordPromptRect = rect{x: px, y: py, w: pw, h: ph}
	a.passwordInputRect = rect{x: px + 20, y: py + 84, w: pw - 40, h: 34}
	a.passwordSubmitRect = rect{x: px + pw - 186, y: py + ph - 46, w: 80, h: 30}
	a.passwordCancelRect = rect{x: px + pw - 96, y: py + ph - 46, w: 80, h: 30}
}

func (a *App) drawPasswordPrompt(screen *ebiten.Image, w, h int) {
	if !a.showPasswordPrompt {
		return
	}
	a.layoutPasswordPromptBounds(w, h)

	overlay := color.RGBA{R: 0, G: 0, B: 0, A: 90}
	a.drawFilledRectOnScreen(screen, 0, 0, w, h, overlay)

	r := a.passwordPromptRect
	a.drawFilledRectOnScreen(screen, r.x, r.y, r.w, r.h, color.RGBA{R: 249, G: 251, B: 254, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y), float64(r.x+r.w), float64(r.y), color.RGBA{R: 160, G: 176, B: 198, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y+r.h), float64(r.x+r.w), float64(r.y+r.h), color.RGBA{R: 160, G: 176, B: 198, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y), float64(r.x), float64(r.y+r.h), color.RGBA{R: 160, G: 176, B: 198, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x+r.w), float64(r.y), float64(r.x+r.w), float64(r.y+r.h), color.RGBA{R: 160, G: 176, B: 198, A: 255})

	titleFace := a.uiFace(12, true, false)
	labelFace := a.uiFace(10, false, false)
	text.Draw(screen, "Password Required", titleFace, r.x+20, r.y+30, color.RGBA{R: 24, G: 38, B: 56, A: 255})
	fileLabel := "File: " + filepath.Base(a.passwordPromptPath)
	text.Draw(screen, fileLabel, labelFace, r.x+20, r.y+54, color.RGBA{R: 52, G: 66, B: 92, A: 255})
	text.Draw(screen, "Enter password to open this encrypted SQDoc:", labelFace, r.x+20, r.y+74, color.RGBA{R: 52, G: 66, B: 92, A: 255})

	inputBg := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	inputBorder := color.RGBA{R: 170, G: 184, B: 202, A: 255}
	if a.passwordPromptFocused {
		inputBg = color.RGBA{R: 244, G: 249, B: 255, A: 255}
		inputBorder = color.RGBA{R: 77, G: 134, B: 205, A: 255}
	}
	a.drawFilledRectOnScreen(screen, a.passwordInputRect.x, a.passwordInputRect.y, a.passwordInputRect.w, a.passwordInputRect.h, inputBg)
	ebitenutil.DrawLine(screen, float64(a.passwordInputRect.x), float64(a.passwordInputRect.y), float64(a.passwordInputRect.x+a.passwordInputRect.w), float64(a.passwordInputRect.y), inputBorder)
	ebitenutil.DrawLine(screen, float64(a.passwordInputRect.x), float64(a.passwordInputRect.y+a.passwordInputRect.h), float64(a.passwordInputRect.x+a.passwordInputRect.w), float64(a.passwordInputRect.y+a.passwordInputRect.h), inputBorder)
	ebitenutil.DrawLine(screen, float64(a.passwordInputRect.x), float64(a.passwordInputRect.y), float64(a.passwordInputRect.x), float64(a.passwordInputRect.y+a.passwordInputRect.h), inputBorder)
	ebitenutil.DrawLine(screen, float64(a.passwordInputRect.x+a.passwordInputRect.w), float64(a.passwordInputRect.y), float64(a.passwordInputRect.x+a.passwordInputRect.w), float64(a.passwordInputRect.y+a.passwordInputRect.h), inputBorder)

	masked := strings.Repeat("*", utf8.RuneCountInString(a.passwordPromptInput))
	text.Draw(screen, masked, labelFace, a.passwordInputRect.x+8, a.passwordInputRect.y+22, color.RGBA{R: 42, G: 56, B: 80, A: 255})
	if a.passwordPromptFocused && (a.frameTick/30)%2 == 0 {
		caretX := a.passwordInputRect.x + 8 + a.measureString(labelFace, masked)
		ebitenutil.DrawLine(screen, float64(caretX), float64(a.passwordInputRect.y+7), float64(caretX), float64(a.passwordInputRect.y+a.passwordInputRect.h-7), color.RGBA{R: 21, G: 84, B: 164, A: 255})
	}

	if a.passwordPromptError != "" {
		text.Draw(screen, a.passwordPromptError, labelFace, r.x+20, a.passwordInputRect.y+a.passwordInputRect.h+22, color.RGBA{R: 165, G: 35, B: 35, A: 255})
	}

	a.drawFilledRectOnScreen(screen, a.passwordSubmitRect.x, a.passwordSubmitRect.y, a.passwordSubmitRect.w, a.passwordSubmitRect.h, color.RGBA{R: 217, G: 233, B: 250, A: 255})
	a.drawFilledRectOnScreen(screen, a.passwordCancelRect.x, a.passwordCancelRect.y, a.passwordCancelRect.w, a.passwordCancelRect.h, color.RGBA{R: 236, G: 241, B: 248, A: 255})
	text.Draw(screen, "Open", labelFace, a.passwordSubmitRect.x+24, a.passwordSubmitRect.y+20, color.RGBA{R: 30, G: 66, B: 118, A: 255})
	text.Draw(screen, "Cancel", labelFace, a.passwordCancelRect.x+20, a.passwordCancelRect.y+20, color.RGBA{R: 52, G: 66, B: 92, A: 255})
}

func (a *App) hitTestPosition(x, y int) (int, int) {
	if len(a.lineLayouts) == 0 {
		return a.state.CurrentBlock, a.state.CaretByte
	}
	first := a.lineLayouts[0]
	if y <= first.y {
		return first.block, first.startByte + a.byteAtX(first, x-first.viewX)
	}
	for _, ll := range a.lineLayouts {
		if y >= ll.y && y <= ll.y+ll.height {
			return ll.block, ll.startByte + a.byteAtX(ll, x-ll.viewX)
		}
	}
	last := a.lineLayouts[len(a.lineLayouts)-1]
	return last.block, last.startByte + a.byteAtX(last, x-last.viewX)
}

func (a *App) lineAdvance(line lineLayout, relByte int) int {
	if relByte <= 0 {
		return 0
	}
	if relByte >= len(line.text) {
		return line.width
	}
	advance := 0
	for _, seg := range line.segments {
		if relByte >= seg.end {
			advance += seg.width
			continue
		}
		if relByte <= seg.start {
			break
		}
		part := string(line.text[seg.start:relByte])
		advance += a.measureString(seg.face, part)
		break
	}
	return advance
}

func (a *App) byteAtX(line lineLayout, relX int) int {
	if relX <= 0 {
		return 0
	}
	x := 0
	for _, seg := range line.segments {
		if relX > x+seg.width {
			x += seg.width
			continue
		}
		bytesSeg := line.text[seg.start:seg.end]
		pos := seg.start
		runX := x
		for len(bytesSeg) > 0 {
			r, size := utf8.DecodeRune(bytesSeg)
			if size <= 0 {
				size = 1
			}
			rw := a.measureString(seg.face, string(r))
			if relX < runX+rw/2 {
				return pos
			}
			runX += rw
			pos += size
			bytesSeg = bytesSeg[size:]
		}
		return seg.end
	}
	return len(line.text)
}

func (a *App) clampScroll() {
	if a.scrollX < 0 {
		a.scrollX = 0
	}
	if a.scrollY < 0 {
		a.scrollY = 0
	}
	if a.scrollX > a.maxX {
		a.scrollX = a.maxX
	}
	if a.scrollY > a.maxY {
		a.scrollY = a.maxY
	}
}

func (a *App) ensureCaretVisible() {
	if len(a.lineLayouts) == 0 || a.contentRect.h <= 0 {
		return
	}
	block := a.state.CurrentBlock
	caret := a.state.CaretByte
	for _, ll := range a.lineLayouts {
		if ll.block != block {
			continue
		}
		lineStart := ll.startByte
		lineEnd := lineStart + len(ll.text)
		if caret < lineStart || caret > lineEnd {
			continue
		}
		top := float64(ll.docY)
		bottom := float64(ll.docY + ll.height)
		viewTop := a.scrollY
		viewBottom := a.scrollY + float64(a.contentRect.h)
		if top < viewTop {
			a.scrollY = top
		}
		if bottom > viewBottom {
			a.scrollY = bottom - float64(a.contentRect.h)
		}

		rel := caret - lineStart
		caretDocX := float64(ll.docX + a.lineAdvance(ll, rel))
		viewLeft := a.scrollX
		viewRight := a.scrollX + float64(a.contentRect.w-12)
		padding := 16.0
		if caretDocX < viewLeft+padding {
			a.scrollX = math.Max(0, caretDocX-padding)
		}
		if caretDocX > viewRight-padding {
			a.scrollX = caretDocX - float64(a.contentRect.w-12) + padding
		}
		break
	}
	a.clampScroll()
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
	path = filepath.Clean(path)
	env, err := sqdoc.InspectEnvelope(path)
	if err != nil {
		return err
	}
	a.applyEnvelopeSettings(env)
	if env.Encrypted && strings.TrimSpace(a.encryptionPassword) == "" {
		a.showPasswordPrompt = true
		a.passwordPromptFocused = true
		a.passwordPromptPath = path
		a.passwordPromptInput = ""
		a.passwordPromptError = ""
		a.dragSelecting = false
		a.status = "Password required to open encrypted document"
		return nil
	}

	doc, err := sqdoc.LoadWithOptions(path, sqdoc.LoadOptions{Password: a.encryptionPassword})
	if err != nil {
		if errors.Is(err, sqdoc.ErrPasswordRequired) || errors.Is(err, sqdoc.ErrInvalidPassword) {
			a.showPasswordPrompt = true
			a.passwordPromptFocused = true
			a.passwordPromptPath = path
			a.passwordPromptInput = ""
			a.passwordPromptError = ""
			a.dragSelecting = false
			a.status = "Password required to open encrypted document"
			if errors.Is(err, sqdoc.ErrInvalidPassword) {
				a.passwordPromptError = "Incorrect password. Enter password to open."
			}
			return nil
		}
		return err
	}
	a.state = editor.NewState(doc)
	a.filePath = path
	a.status = "Opened " + filepath.Base(path)
	a.applyEnvelopeSettings(env)
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
	prev := a.uiScaleIdx
	a.uiScaleIdx += delta
	if a.uiScaleIdx < 0 {
		a.uiScaleIdx = 0
	}
	if a.uiScaleIdx >= len(a.uiScales) {
		a.uiScaleIdx = len(a.uiScales) - 1
	}
	if prev != a.uiScaleIdx {
		a.fonts.cache = map[fontKey]font.Face{}
	}
}

func (a *App) layoutHelpDialogBounds(w, h int) {
	panelW := int(float64(w) * 0.68)
	panelH := int(float64(h) * 0.68)
	if panelW > w-40 {
		panelW = w - 40
	}
	if panelH > h-40 {
		panelH = h - 40
	}
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	a.helpRect = rect{x: px, y: py, w: panelW, h: panelH}
	a.helpClose = rect{x: px + panelW - 94, y: py + 12, w: 78, h: 30}
}

func (a *App) drawHelpOverlay(screen *ebiten.Image, face font.Face) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	a.layoutHelpDialogBounds(w, h)
	r := a.helpRect
	a.drawFilledRectOnScreen(screen, 0, 0, w, h, color.RGBA{R: 0, G: 0, B: 0, A: 90})
	a.drawFilledRectOnScreen(screen, r.x, r.y, r.w, r.h, color.RGBA{R: 250, G: 251, B: 253, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y), float64(r.x+r.w), float64(r.y), color.RGBA{R: 170, G: 184, B: 202, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y+r.h), float64(r.x+r.w), float64(r.y+r.h), color.RGBA{R: 170, G: 184, B: 202, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x), float64(r.y), float64(r.x), float64(r.y+r.h), color.RGBA{R: 170, G: 184, B: 202, A: 255})
	ebitenutil.DrawLine(screen, float64(r.x+r.w), float64(r.y), float64(r.x+r.w), float64(r.y+r.h), color.RGBA{R: 170, G: 184, B: 202, A: 255})

	a.drawFilledRectOnScreen(screen, a.helpClose.x, a.helpClose.y, a.helpClose.w, a.helpClose.h, color.RGBA{R: 236, G: 241, B: 248, A: 255})
	text.Draw(screen, "Close", face, a.helpClose.x+22, a.helpClose.y+20, color.RGBA{R: 52, G: 66, B: 92, A: 255})

	titleFace := a.uiFace(12, true, false)
	text.Draw(screen, "Help", titleFace, r.x+22, r.y+30, color.RGBA{R: 30, G: 45, B: 67, A: 255})

	lines := []string{
		"Ctrl+S: Save | Ctrl+Shift+S: Save As",
		"Ctrl+O: Open | Ctrl+N: New",
		"Ctrl+Z: Undo | Ctrl+Y: Redo",
		"Ctrl+B/I/U: Bold / Italic / Underline",
		"Ctrl+Shift+H: Toggle text highlight",
		"Ctrl+Backspace / Ctrl+Delete: Delete previous/next word",
		"Mouse wheel: vertical scroll | Shift+wheel: horizontal",
		"Click inside document to set caret; drag to select",
		"F1 or Esc closes this dialog",
	}
	y := r.y + 62
	labelFace := a.uiFace(10, false, false)
	for _, l := range lines {
		text.Draw(screen, l, labelFace, r.x+20, y, color.RGBA{R: 48, G: 60, B: 78, A: 255})
		y += int(24 * a.uiScales[a.uiScaleIdx])
	}
}

func (a *App) fillRectWithinContent(x, y, w, h int, c color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	cx, cy, cw, ch := a.contentRect.x, a.contentRect.y, a.contentRect.w, a.contentRect.h
	if x < cx {
		w -= cx - x
		x = cx
	}
	if y < cy {
		h -= cy - y
		y = cy
	}
	if x+w > cx+cw {
		w = cx + cw - x
	}
	if y+h > cy+ch {
		h = cy + ch - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	a.frameBuffer.FillRect(x, y, w, h, c)
}

func defaultAttr() sqdoc.StyleAttr {
	return sqdoc.StyleAttr{FontSizePt: 14, ColorRGBA: 0x202020FF}
}

func rgbaFromUint32(u uint32) color.RGBA {
	return color.RGBA{R: uint8((u >> 24) & 0xFF), G: uint8((u >> 16) & 0xFF), B: uint8((u >> 8) & 0xFF), A: uint8(u & 0xFF)}
}
