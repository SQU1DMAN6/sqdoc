package editor

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"sqdoc/pkg/sqdoc"
)

type Position struct {
	Block int
	Byte  int
}

type State struct {
	Doc          *sqdoc.Document
	CurrentBlock int
	CaretByte    int
	ZoomPercent  int
	UIScale      int
	HelpVisible  bool

	selectionAnchor    Position
	selectionAnchored  bool
	selectionIsVisible bool
}

func NewState(doc *sqdoc.Document) *State {
	if doc == nil {
		doc = sqdoc.NewDocument("", "Untitled")
	}
	s := &State{Doc: doc, CurrentBlock: 0, ZoomPercent: 100, UIScale: 100}
	s.Normalize()
	return s
}

func (s *State) Normalize() {
	s.ensureDocument()
	for i := range s.Doc.Blocks {
		if s.Doc.Blocks[i].Kind != sqdoc.BlockKindText {
			continue
		}
		if s.Doc.Blocks[i].Text == nil {
			s.Doc.Blocks[i].Text = &sqdoc.TextBlock{}
		}
		s.sanitizeBlockRuns(i)
	}
	if len(s.Doc.Blocks) == 0 {
		s.Doc.Blocks = append(s.Doc.Blocks, sqdoc.Block{
			ID:   1,
			Kind: sqdoc.BlockKindText,
			Text: &sqdoc.TextBlock{UTF8: []byte{}, Runs: []sqdoc.StyleRun{{Start: 0, End: 0, Attr: defaultStyleAttr()}}},
		})
	}
	if s.CurrentBlock < 0 {
		s.CurrentBlock = 0
	}
	if s.CurrentBlock >= len(s.Doc.Blocks) {
		s.CurrentBlock = len(s.Doc.Blocks) - 1
	}
	s.CaretByte = clampToRuneBoundary(s.CurrentBlockText(), s.CaretByte)
	if s.selectionAnchored {
		s.selectionAnchor = s.clampPosition(s.selectionAnchor)
		s.selectionIsVisible = comparePos(s.selectionAnchor, s.caretPos()) != 0
	}
}

func (s *State) BlockCount() int {
	if s.Doc == nil {
		return 0
	}
	return len(s.Doc.Blocks)
}

func (s *State) AddTextBlock(text string) uint64 {
	s.ensureDocument()
	id := nextBlockID(s.Doc.Blocks)
	attr := s.currentStyleAttr()
	tb := &sqdoc.TextBlock{UTF8: []byte(text)}
	if len(tb.UTF8) == 0 {
		tb.Runs = []sqdoc.StyleRun{{Start: 0, End: 0, Attr: normalizeAttr(attr)}}
	} else {
		tb.Runs = []sqdoc.StyleRun{{Start: 0, End: uint32(len(tb.UTF8)), Attr: normalizeAttr(attr)}}
	}
	s.Doc.Blocks = append(s.Doc.Blocks, sqdoc.Block{ID: id, Kind: sqdoc.BlockKindText, Text: tb})
	s.CurrentBlock = len(s.Doc.Blocks) - 1
	s.CaretByte = len(text)
	s.ClearSelection()
	return id
}

func (s *State) CurrentText() string {
	return string(s.CurrentBlockText())
}

func (s *State) UpdateCurrentText(text string) error {
	if !utf8.ValidString(text) {
		return fmt.Errorf("text must be valid UTF-8")
	}
	s.Normalize()
	insertAttr := s.currentStyleAttr()
	tb := s.currentBlockTextRef()
	tb.UTF8 = []byte(text)
	if len(tb.UTF8) == 0 {
		tb.Runs = []sqdoc.StyleRun{{Start: 0, End: 0, Attr: normalizeAttr(insertAttr)}}
	} else {
		tb.Runs = []sqdoc.StyleRun{{Start: 0, End: uint32(len(tb.UTF8)), Attr: normalizeAttr(insertAttr)}}
	}
	s.CaretByte = len(text)
	s.ClearSelection()
	return nil
}

func (s *State) SetCurrentBlock(index int) {
	s.Normalize()
	if index < 0 {
		index = 0
	}
	if index >= len(s.Doc.Blocks) {
		index = len(s.Doc.Blocks) - 1
	}
	s.CurrentBlock = index
	s.CaretByte = clampToRuneBoundary(s.CurrentBlockText(), s.CaretByte)
}

func (s *State) SetCaret(block, bytePos int) {
	s.Normalize()
	if block < 0 {
		block = 0
	}
	if block >= len(s.Doc.Blocks) {
		block = len(s.Doc.Blocks) - 1
	}
	txt := blockText(s.Doc.Blocks[block])
	bytePos = clampToRuneBoundary(txt, bytePos)
	s.CurrentBlock = block
	s.CaretByte = bytePos
	if s.selectionAnchored {
		s.selectionIsVisible = comparePos(s.selectionAnchor, s.caretPos()) != 0
	}
}

func (s *State) MoveBlock(delta int) {
	s.SetCurrentBlock(s.CurrentBlock + delta)
}

func (s *State) MoveCaretLeft() {
	s.Normalize()
	text := s.CurrentBlockText()
	if s.CaretByte <= 0 {
		if s.CurrentBlock > 0 {
			s.CurrentBlock--
			s.CaretByte = len(s.CurrentBlockText())
		}
		return
	}
	_, size := utf8.DecodeLastRune(text[:s.CaretByte])
	if size <= 0 {
		size = 1
	}
	s.CaretByte -= size
	s.CaretByte = clampToRuneBoundary(text, s.CaretByte)
}

func (s *State) MoveCaretRight() {
	s.Normalize()
	text := s.CurrentBlockText()
	if s.CaretByte >= len(text) {
		if s.CurrentBlock < len(s.Doc.Blocks)-1 {
			s.CurrentBlock++
			s.CaretByte = 0
		}
		return
	}
	_, size := utf8.DecodeRune(text[s.CaretByte:])
	if size <= 0 {
		size = 1
	}
	s.CaretByte += size
	s.CaretByte = clampToRuneBoundary(text, s.CaretByte)
}

func (s *State) MoveCaretWordLeft() {
	s.Normalize()
	text := s.CurrentBlockText()
	if s.CaretByte <= 0 {
		if s.CurrentBlock > 0 {
			s.CurrentBlock--
			s.CaretByte = len(s.CurrentBlockText())
		}
		return
	}
	pos := s.CaretByte
	for pos > 0 {
		r, size := utf8.DecodeLastRune(text[:pos])
		if size <= 0 {
			size = 1
		}
		if isWordRune(r) {
			break
		}
		pos -= size
	}
	for pos > 0 {
		r, size := utf8.DecodeLastRune(text[:pos])
		if size <= 0 {
			size = 1
		}
		if !isWordRune(r) {
			break
		}
		pos -= size
	}
	s.CaretByte = clampToRuneBoundary(text, pos)
}

func (s *State) MoveCaretWordRight() {
	s.Normalize()
	text := s.CurrentBlockText()
	if s.CaretByte >= len(text) {
		if s.CurrentBlock < len(s.Doc.Blocks)-1 {
			s.CurrentBlock++
			s.CaretByte = 0
		}
		return
	}
	pos := s.CaretByte
	for pos < len(text) {
		r, size := utf8.DecodeRune(text[pos:])
		if size <= 0 {
			size = 1
		}
		if isWordRune(r) {
			break
		}
		pos += size
	}
	for pos < len(text) {
		r, size := utf8.DecodeRune(text[pos:])
		if size <= 0 {
			size = 1
		}
		if !isWordRune(r) {
			break
		}
		pos += size
	}
	s.CaretByte = clampToRuneBoundary(text, pos)
}

func (s *State) MoveCaretToLineStart() {
	s.Normalize()
	s.CaretByte = 0
}

func (s *State) MoveCaretToLineEnd() {
	s.Normalize()
	s.CaretByte = len(s.CurrentBlockText())
}

func (s *State) InsertTextAtCaret(input string) error {
	if input == "" {
		return nil
	}
	if !utf8.ValidString(input) {
		return fmt.Errorf("input must be valid UTF-8")
	}
	s.Normalize()
	input = strings.ReplaceAll(input, "\r\n", "\n")

	if s.HasSelection() {
		s.DeleteSelection()
		s.Normalize()
	}

	text := s.CurrentBlockText()
	pos := clampToRuneBoundary(text, s.CaretByte)
	insertAttr := s.currentStyleAttr()
	parts := strings.Split(input, "\n")
	if len(parts) == 1 {
		s.replaceRangeInBlock(s.CurrentBlock, pos, pos, []byte(parts[0]), insertAttr)
		s.CaretByte = pos + len(parts[0])
		s.ClearSelection()
		return nil
	}

	oldText := append([]byte(nil), s.CurrentBlockText()...)
	rightText := append([]byte(nil), oldText[pos:]...)
	rightRuns := s.clipBlockRuns(s.CurrentBlock, pos, len(oldText), 0)

	s.replaceRangeInBlock(s.CurrentBlock, pos, len(oldText), []byte(parts[0]), insertAttr)

	insertAt := s.CurrentBlock
	for i := 1; i < len(parts); i++ {
		segText := []byte(parts[i])
		segRuns := []sqdoc.StyleRun{}
		if len(segText) == 0 {
			segRuns = []sqdoc.StyleRun{{Start: 0, End: 0, Attr: normalizeAttr(insertAttr)}}
		} else {
			segRuns = []sqdoc.StyleRun{{Start: 0, End: uint32(len(segText)), Attr: normalizeAttr(insertAttr)}}
		}
		if i == len(parts)-1 {
			shift := len(segText)
			mergedText := append(append([]byte(nil), segText...), rightText...)
			mergedRuns := append([]sqdoc.StyleRun{}, segRuns...)
			for _, r := range rightRuns {
				mergedRuns = append(mergedRuns, sqdoc.StyleRun{
					Start: uint32(int(r.Start) + shift),
					End:   uint32(int(r.End) + shift),
					Attr:  normalizeAttr(r.Attr),
				})
			}
			segText = mergedText
			segRuns = sanitizeRuns(len(segText), mergedRuns)
		}

		newID := nextBlockID(s.Doc.Blocks)
		newBlock := sqdoc.Block{ID: newID, Kind: sqdoc.BlockKindText, Text: &sqdoc.TextBlock{UTF8: segText, Runs: segRuns}}
		s.Doc.Blocks = append(s.Doc.Blocks, sqdoc.Block{})
		copy(s.Doc.Blocks[insertAt+2:], s.Doc.Blocks[insertAt+1:])
		s.Doc.Blocks[insertAt+1] = newBlock
		insertAt++
	}

	s.CurrentBlock = insertAt
	s.CaretByte = len(parts[len(parts)-1])
	s.ClearSelection()
	return nil
}

func (s *State) SplitBlockAtCaret() {
	_ = s.InsertTextAtCaret("\n")
}

func (s *State) Backspace() {
	s.Normalize()
	if s.DeleteSelection() {
		return
	}

	text := s.CurrentBlockText()
	if s.CaretByte > 0 {
		start := previousRuneBoundary(text, s.CaretByte)
		insertAttr := s.styleAt(s.CurrentBlock, start)
		s.replaceRangeInBlock(s.CurrentBlock, start, s.CaretByte, nil, insertAttr)
		s.CaretByte = start
		return
	}

	if s.CurrentBlock == 0 {
		return
	}
	oldIdx := s.CurrentBlock
	prevLen := len(blockText(s.Doc.Blocks[oldIdx-1]))
	s.mergeBlocks(oldIdx-1, oldIdx)
	s.CurrentBlock--
	s.CaretByte = prevLen
}

func (s *State) DeleteForward() {
	s.Normalize()
	if s.DeleteSelection() {
		return
	}

	text := s.CurrentBlockText()
	if s.CaretByte < len(text) {
		end := nextRuneBoundary(text, s.CaretByte)
		insertAttr := s.styleAt(s.CurrentBlock, s.CaretByte)
		s.replaceRangeInBlock(s.CurrentBlock, s.CaretByte, end, nil, insertAttr)
		return
	}

	if s.CurrentBlock >= len(s.Doc.Blocks)-1 {
		return
	}
	s.mergeBlocks(s.CurrentBlock, s.CurrentBlock+1)
}

func (s *State) DeleteWordBackward() {
	s.Normalize()
	if s.DeleteSelection() {
		return
	}

	text := s.CurrentBlockText()
	if s.CaretByte == 0 {
		if s.CurrentBlock > 0 {
			oldIdx := s.CurrentBlock
			prevLen := len(blockText(s.Doc.Blocks[oldIdx-1]))
			s.mergeBlocks(oldIdx-1, oldIdx)
			s.CurrentBlock--
			s.CaretByte = prevLen
		}
		return
	}

	start := previousWordBoundary(text, s.CaretByte)
	insertAttr := s.styleAt(s.CurrentBlock, start)
	s.replaceRangeInBlock(s.CurrentBlock, start, s.CaretByte, nil, insertAttr)
	s.CaretByte = start
}

func (s *State) DeleteWordForward() {
	s.Normalize()
	if s.DeleteSelection() {
		return
	}

	text := s.CurrentBlockText()
	if s.CaretByte >= len(text) {
		if s.CurrentBlock < len(s.Doc.Blocks)-1 {
			s.mergeBlocks(s.CurrentBlock, s.CurrentBlock+1)
		}
		return
	}

	end := nextWordBoundary(text, s.CaretByte)
	insertAttr := s.styleAt(s.CurrentBlock, s.CaretByte)
	s.replaceRangeInBlock(s.CurrentBlock, s.CaretByte, end, nil, insertAttr)
}

func (s *State) ToggleBold() {
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) { attr.Bold = !attr.Bold })
}

func (s *State) ToggleItalic() {
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) { attr.Italic = !attr.Italic })
}

func (s *State) ToggleUnderline() {
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) { attr.Underline = !attr.Underline })
}

func (s *State) ToggleHighlight() {
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) { attr.Highlight = !attr.Highlight })
}

func (s *State) IncreaseFontSize() {
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) {
		if attr.FontSizePt < 96 {
			attr.FontSizePt++
		}
	})
}

func (s *State) DecreaseFontSize() {
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) {
		if attr.FontSizePt > 8 {
			attr.FontSizePt--
		}
	})
}

func (s *State) CycleColor() {
	palette := []uint32{
		0x202020FF,
		0x0057B8FF,
		0xA31515FF,
		0x117A37FF,
		0x7A2DB8FF,
	}
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) {
		idx := 0
		for i := range palette {
			if palette[i] == attr.ColorRGBA {
				idx = i
				break
			}
		}
		attr.ColorRGBA = palette[(idx+1)%len(palette)]
	})
}

func (s *State) SetColor(rgba uint32) {
	if rgba == 0 {
		rgba = defaultStyleAttr().ColorRGBA
	}
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) {
		attr.ColorRGBA = rgba
	})
}

func (s *State) SetFontSize(pt uint16) {
	if pt < 8 {
		pt = 8
	}
	if pt > 96 {
		pt = 96
	}
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) {
		attr.FontSizePt = pt
	})
}

func (s *State) SetFontFamily(family sqdoc.FontFamily) {
	if !isValidFontFamily(family) {
		family = sqdoc.FontFamilySans
	}
	s.applyStyleMutation(func(attr *sqdoc.StyleAttr) {
		attr.FontFamily = family
	})
}

func (s *State) CurrentStyleAttr() sqdoc.StyleAttr {
	return s.currentStyleAttr()
}

func (s *State) BlockStyleAttr(index int) sqdoc.StyleAttr {
	s.Normalize()
	if index < 0 || index >= len(s.Doc.Blocks) {
		return defaultStyleAttr()
	}
	return s.styleAt(index, 0)
}

func (s *State) BlockRuns(index int) []sqdoc.StyleRun {
	s.Normalize()
	if s.Doc == nil || index < 0 || index >= len(s.Doc.Blocks) {
		return nil
	}
	tb := s.Doc.Blocks[index].Text
	if tb == nil {
		return nil
	}
	cov := coverageRuns(len(tb.UTF8), tb.Runs)
	out := make([]sqdoc.StyleRun, len(cov))
	copy(out, cov)
	return out
}

func (s *State) CurrentBlockText() []byte {
	if s.Doc == nil || s.CurrentBlock < 0 || s.CurrentBlock >= len(s.Doc.Blocks) {
		return nil
	}
	block := &s.Doc.Blocks[s.CurrentBlock]
	if block.Text == nil {
		block.Text = &sqdoc.TextBlock{}
	}
	return block.Text.UTF8
}

func (s *State) HasSelection() bool {
	s.Normalize()
	return s.selectionIsVisible
}

func (s *State) EnsureSelectionAnchor() {
	s.Normalize()
	if s.selectionAnchored {
		return
	}
	s.selectionAnchor = s.caretPos()
	s.selectionAnchored = true
	s.selectionIsVisible = false
}

func (s *State) UpdateSelectionFromCaret() {
	s.Normalize()
	if !s.selectionAnchored {
		s.selectionAnchor = s.caretPos()
		s.selectionAnchored = true
	}
	s.selectionIsVisible = comparePos(s.selectionAnchor, s.caretPos()) != 0
}

func (s *State) ClearSelection() {
	s.selectionAnchored = false
	s.selectionIsVisible = false
}

func (s *State) SelectionRange() (Position, Position, bool) {
	s.Normalize()
	if !s.selectionIsVisible {
		return Position{}, Position{}, false
	}
	a := s.selectionAnchor
	b := s.caretPos()
	if comparePos(a, b) <= 0 {
		return a, b, true
	}
	return b, a, true
}

func (s *State) SelectAll() {
	s.Normalize()
	if len(s.Doc.Blocks) == 0 {
		return
	}
	s.selectionAnchor = Position{Block: 0, Byte: 0}
	s.selectionAnchored = true
	last := len(s.Doc.Blocks) - 1
	s.CurrentBlock = last
	s.CaretByte = len(blockText(s.Doc.Blocks[last]))
	s.selectionIsVisible = comparePos(s.selectionAnchor, s.caretPos()) != 0
}

func (s *State) SelectedText() string {
	start, end, ok := s.SelectionRange()
	if !ok {
		return ""
	}
	if start.Block == end.Block {
		b := blockText(s.Doc.Blocks[start.Block])
		return string(b[start.Byte:end.Byte])
	}

	var out strings.Builder
	first := blockText(s.Doc.Blocks[start.Block])
	out.Write(first[start.Byte:])
	for i := start.Block + 1; i < end.Block; i++ {
		out.WriteByte('\n')
		out.Write(blockText(s.Doc.Blocks[i]))
	}
	out.WriteByte('\n')
	last := blockText(s.Doc.Blocks[end.Block])
	out.Write(last[:end.Byte])
	return out.String()
}

func (s *State) DeleteSelection() bool {
	start, end, ok := s.SelectionRange()
	if !ok {
		return false
	}
	if comparePos(start, end) >= 0 {
		s.ClearSelection()
		return false
	}

	if start.Block == end.Block {
		insertAttr := s.styleAt(start.Block, start.Byte)
		s.replaceRangeInBlock(start.Block, start.Byte, end.Byte, nil, insertAttr)
		s.CurrentBlock = start.Block
		s.CaretByte = start.Byte
		s.ClearSelection()
		return true
	}

	leftPrefix := append([]byte(nil), blockText(s.Doc.Blocks[start.Block])[:start.Byte]...)
	rightSuffix := append([]byte(nil), blockText(s.Doc.Blocks[end.Block])[end.Byte:]...)
	merged := append(leftPrefix, rightSuffix...)

	leftRuns := s.clipBlockRuns(start.Block, 0, start.Byte, 0)
	rightRuns := s.clipBlockRuns(end.Block, end.Byte, len(blockText(s.Doc.Blocks[end.Block])), start.Byte)
	newRuns := append(leftRuns, rightRuns...)
	if len(merged) == 0 {
		newRuns = []sqdoc.StyleRun{{Start: 0, End: 0, Attr: normalizeAttr(s.styleAt(start.Block, start.Byte))}}
	} else {
		newRuns = sanitizeRuns(len(merged), newRuns)
	}

	s.Doc.Blocks[start.Block].Text.UTF8 = merged
	s.Doc.Blocks[start.Block].Text.Runs = newRuns
	s.Doc.Blocks = append(s.Doc.Blocks[:start.Block+1], s.Doc.Blocks[end.Block+1:]...)
	s.CurrentBlock = start.Block
	s.CaretByte = start.Byte
	s.Normalize()
	s.ClearSelection()
	return true
}

func (s *State) ensureDocument() {
	if s.Doc == nil {
		s.Doc = sqdoc.NewDocument("", "Untitled")
	}
}

func (s *State) AllBlockTexts() []string {
	s.Normalize()
	out := make([]string, 0, len(s.Doc.Blocks))
	for i := range s.Doc.Blocks {
		tb := s.Doc.Blocks[i].Text
		if tb == nil {
			out = append(out, "")
			continue
		}
		out = append(out, string(tb.UTF8))
	}
	return out
}

func (s *State) SortBlocksByID() {
	if s.Doc == nil {
		return
	}
	sort.Slice(s.Doc.Blocks, func(i, j int) bool { return s.Doc.Blocks[i].ID < s.Doc.Blocks[j].ID })
	s.Normalize()
	s.ClearSelection()
}

func (s *State) currentStyleAttr() sqdoc.StyleAttr {
	start, _, has := s.SelectionRange()
	if has {
		return s.styleAt(start.Block, start.Byte)
	}
	return s.styleAt(s.CurrentBlock, s.CaretByte)
}

func (s *State) applyStyleMutation(mut func(*sqdoc.StyleAttr)) {
	s.Normalize()
	if mut == nil {
		return
	}
	if start, end, has := s.SelectionRange(); has {
		for b := start.Block; b <= end.Block; b++ {
			segStart := 0
			segEnd := len(blockText(s.Doc.Blocks[b]))
			if b == start.Block {
				segStart = start.Byte
			}
			if b == end.Block {
				segEnd = end.Byte
			}
			s.applyStyleToBlockRange(b, segStart, segEnd, mut)
		}
		return
	}

	txt := s.CurrentBlockText()
	if len(txt) == 0 {
		s.applyStyleToBlockRange(s.CurrentBlock, 0, 0, mut)
		return
	}
	pos := s.CaretByte
	if pos >= len(txt) {
		pos = previousRuneBoundary(txt, len(txt))
	}
	end := nextRuneBoundary(txt, pos)
	s.applyStyleToBlockRange(s.CurrentBlock, pos, end, mut)
}

func (s *State) applyStyleToBlockRange(blockIndex, start, end int, mut func(*sqdoc.StyleAttr)) {
	if s.Doc == nil || blockIndex < 0 || blockIndex >= len(s.Doc.Blocks) {
		return
	}
	tb := s.Doc.Blocks[blockIndex].Text
	if tb == nil {
		tb = &sqdoc.TextBlock{}
		s.Doc.Blocks[blockIndex].Text = tb
	}
	textLen := len(tb.UTF8)
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > textLen {
		start = textLen
	}
	if end > textLen {
		end = textLen
	}
	if start > end {
		start, end = end, start
	}
	if textLen == 0 {
		attr := normalizeAttr(s.styleAt(blockIndex, 0))
		mut(&attr)
		tb.Runs = []sqdoc.StyleRun{{Start: 0, End: 0, Attr: normalizeAttr(attr)}}
		return
	}
	if start == end {
		if start >= textLen {
			start = previousRuneBoundary(tb.UTF8, textLen)
		}
		end = nextRuneBoundary(tb.UTF8, start)
	}

	cov := coverageRuns(textLen, tb.Runs)
	newRuns := make([]sqdoc.StyleRun, 0, len(cov)+2)
	for _, r := range cov {
		rs := int(r.Start)
		re := int(r.End)
		if re <= start || rs >= end {
			newRuns = append(newRuns, r)
			continue
		}
		if rs < start {
			newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(rs), End: uint32(start), Attr: normalizeAttr(r.Attr)})
			rs = start
		}
		midEnd := min(re, end)
		attr := normalizeAttr(r.Attr)
		mut(&attr)
		newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(rs), End: uint32(midEnd), Attr: normalizeAttr(attr)})
		if re > end {
			newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(end), End: uint32(re), Attr: normalizeAttr(r.Attr)})
		}
	}
	tb.Runs = sanitizeRuns(textLen, newRuns)
}

func (s *State) replaceRangeInBlock(blockIndex, start, end int, insert []byte, insertAttr sqdoc.StyleAttr) {
	if s.Doc == nil || blockIndex < 0 || blockIndex >= len(s.Doc.Blocks) {
		return
	}
	tb := s.Doc.Blocks[blockIndex].Text
	if tb == nil {
		tb = &sqdoc.TextBlock{}
		s.Doc.Blocks[blockIndex].Text = tb
	}
	text := tb.UTF8
	start = clampToRuneBoundary(text, start)
	end = clampToRuneBoundary(text, end)
	if start > end {
		start, end = end, start
	}
	oldLen := len(text)
	cov := coverageRuns(oldLen, tb.Runs)

	newText := make([]byte, 0, oldLen-(end-start)+len(insert))
	newText = append(newText, text[:start]...)
	newText = append(newText, insert...)
	newText = append(newText, text[end:]...)
	delta := len(insert) - (end - start)

	newRuns := make([]sqdoc.StyleRun, 0, len(cov)+2)
	for _, r := range cov {
		rs := int(r.Start)
		re := int(r.End)
		switch {
		case re <= start:
			newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(rs), End: uint32(re), Attr: normalizeAttr(r.Attr)})
		case rs >= end:
			newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(rs + delta), End: uint32(re + delta), Attr: normalizeAttr(r.Attr)})
		default:
			if rs < start {
				newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(rs), End: uint32(start), Attr: normalizeAttr(r.Attr)})
			}
			if re > end {
				newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(end + delta), End: uint32(re + delta), Attr: normalizeAttr(r.Attr)})
			}
		}
	}
	if len(insert) > 0 {
		newRuns = append(newRuns, sqdoc.StyleRun{Start: uint32(start), End: uint32(start + len(insert)), Attr: normalizeAttr(insertAttr)})
	}

	tb.UTF8 = newText
	if len(newText) == 0 {
		tb.Runs = []sqdoc.StyleRun{{Start: 0, End: 0, Attr: normalizeAttr(insertAttr)}}
		return
	}
	tb.Runs = sanitizeRuns(len(newText), newRuns)
}

func (s *State) mergeBlocks(left, right int) {
	if s.Doc == nil || left < 0 || right <= left || right >= len(s.Doc.Blocks) {
		return
	}
	leftText := append([]byte(nil), blockText(s.Doc.Blocks[left])...)
	rightText := append([]byte(nil), blockText(s.Doc.Blocks[right])...)
	leftRuns := s.clipBlockRuns(left, 0, len(leftText), 0)
	rightRuns := s.clipBlockRuns(right, 0, len(rightText), len(leftText))
	mergedText := append(leftText, rightText...)
	mergedRuns := append(leftRuns, rightRuns...)
	if len(mergedText) == 0 {
		mergedRuns = []sqdoc.StyleRun{{Start: 0, End: 0, Attr: defaultStyleAttr()}}
	} else {
		mergedRuns = sanitizeRuns(len(mergedText), mergedRuns)
	}
	s.Doc.Blocks[left].Text.UTF8 = mergedText
	s.Doc.Blocks[left].Text.Runs = mergedRuns
	s.Doc.Blocks = append(s.Doc.Blocks[:right], s.Doc.Blocks[right+1:]...)
}

func (s *State) clipBlockRuns(blockIndex, from, to, shift int) []sqdoc.StyleRun {
	if s.Doc == nil || blockIndex < 0 || blockIndex >= len(s.Doc.Blocks) {
		return nil
	}
	tb := s.Doc.Blocks[blockIndex].Text
	if tb == nil {
		return nil
	}
	textLen := len(tb.UTF8)
	if from < 0 {
		from = 0
	}
	if to < 0 {
		to = 0
	}
	if from > textLen {
		from = textLen
	}
	if to > textLen {
		to = textLen
	}
	if from > to {
		from, to = to, from
	}
	cov := coverageRuns(textLen, tb.Runs)
	out := make([]sqdoc.StyleRun, 0, len(cov))
	for _, r := range cov {
		rs := int(r.Start)
		re := int(r.End)
		if re <= from || rs >= to {
			continue
		}
		if rs < from {
			rs = from
		}
		if re > to {
			re = to
		}
		out = append(out, sqdoc.StyleRun{Start: uint32(rs - from + shift), End: uint32(re - from + shift), Attr: normalizeAttr(r.Attr)})
	}
	return normalizeSparseRuns(out)
}

func (s *State) styleAt(blockIndex, bytePos int) sqdoc.StyleAttr {
	if s.Doc == nil || blockIndex < 0 || blockIndex >= len(s.Doc.Blocks) {
		return defaultStyleAttr()
	}
	tb := s.Doc.Blocks[blockIndex].Text
	if tb == nil {
		return defaultStyleAttr()
	}
	textLen := len(tb.UTF8)
	runs := sanitizeRuns(textLen, tb.Runs)
	if textLen == 0 {
		if len(runs) > 0 {
			return normalizeAttr(runs[0].Attr)
		}
		return defaultStyleAttr()
	}
	if bytePos < 0 {
		bytePos = 0
	}
	if bytePos > textLen {
		bytePos = textLen
	}
	probe := bytePos
	if probe == textLen {
		probe = textLen - 1
	}
	for _, r := range runs {
		if int(r.Start) <= probe && probe < int(r.End) {
			return normalizeAttr(r.Attr)
		}
	}
	return defaultStyleAttr()
}

func (s *State) sanitizeBlockRuns(blockIndex int) {
	if s.Doc == nil || blockIndex < 0 || blockIndex >= len(s.Doc.Blocks) {
		return
	}
	tb := s.Doc.Blocks[blockIndex].Text
	if tb == nil {
		return
	}
	tb.Runs = sanitizeRuns(len(tb.UTF8), tb.Runs)
}

func (s *State) currentBlockTextRef() *sqdoc.TextBlock {
	if s.Doc == nil || s.CurrentBlock < 0 || s.CurrentBlock >= len(s.Doc.Blocks) {
		return &sqdoc.TextBlock{}
	}
	if s.Doc.Blocks[s.CurrentBlock].Text == nil {
		s.Doc.Blocks[s.CurrentBlock].Text = &sqdoc.TextBlock{}
	}
	return s.Doc.Blocks[s.CurrentBlock].Text
}

func (s *State) caretPos() Position {
	return Position{Block: s.CurrentBlock, Byte: s.CaretByte}
}

func (s *State) clampPosition(p Position) Position {
	if s.Doc == nil || len(s.Doc.Blocks) == 0 {
		return Position{Block: 0, Byte: 0}
	}
	if p.Block < 0 {
		p.Block = 0
	}
	if p.Block >= len(s.Doc.Blocks) {
		p.Block = len(s.Doc.Blocks) - 1
	}
	p.Byte = clampToRuneBoundary(blockText(s.Doc.Blocks[p.Block]), p.Byte)
	return p
}

func blockText(b sqdoc.Block) []byte {
	if b.Text == nil {
		return nil
	}
	return b.Text.UTF8
}

func nextBlockID(blocks []sqdoc.Block) uint64 {
	var maxID uint64
	for _, b := range blocks {
		if b.ID > maxID {
			maxID = b.ID
		}
	}
	return maxID + 1
}

func clampToRuneBoundary(text []byte, pos int) int {
	if pos < 0 {
		return 0
	}
	if pos > len(text) {
		pos = len(text)
	}
	if pos == len(text) || utf8.Valid(text[:pos]) {
		return pos
	}
	for pos > 0 && !utf8.Valid(text[:pos]) {
		pos--
	}
	return pos
}

func previousRuneBoundary(text []byte, pos int) int {
	pos = clampToRuneBoundary(text, pos)
	if pos == 0 {
		return 0
	}
	_, size := utf8.DecodeLastRune(text[:pos])
	if size <= 0 {
		size = 1
	}
	return pos - size
}

func nextRuneBoundary(text []byte, pos int) int {
	pos = clampToRuneBoundary(text, pos)
	if pos >= len(text) {
		return len(text)
	}
	_, size := utf8.DecodeRune(text[pos:])
	if size <= 0 {
		size = 1
	}
	return pos + size
}

func previousWordBoundary(text []byte, pos int) int {
	pos = clampToRuneBoundary(text, pos)
	for pos > 0 {
		r, size := utf8.DecodeLastRune(text[:pos])
		if size <= 0 {
			size = 1
		}
		if !unicode.IsSpace(r) {
			break
		}
		pos -= size
	}
	for pos > 0 {
		r, size := utf8.DecodeLastRune(text[:pos])
		if size <= 0 {
			size = 1
		}
		if unicode.IsSpace(r) {
			break
		}
		pos -= size
	}
	return clampToRuneBoundary(text, pos)
}

func nextWordBoundary(text []byte, pos int) int {
	pos = clampToRuneBoundary(text, pos)
	for pos < len(text) {
		r, size := utf8.DecodeRune(text[pos:])
		if size <= 0 {
			size = 1
		}
		if !unicode.IsSpace(r) {
			break
		}
		pos += size
	}
	for pos < len(text) {
		r, size := utf8.DecodeRune(text[pos:])
		if size <= 0 {
			size = 1
		}
		if unicode.IsSpace(r) {
			break
		}
		pos += size
	}
	return clampToRuneBoundary(text, pos)
}

func comparePos(a, b Position) int {
	if a.Block < b.Block {
		return -1
	}
	if a.Block > b.Block {
		return 1
	}
	if a.Byte < b.Byte {
		return -1
	}
	if a.Byte > b.Byte {
		return 1
	}
	return 0
}

func defaultStyleAttr() sqdoc.StyleAttr {
	return sqdoc.StyleAttr{FontSizePt: 14, ColorRGBA: 0x202020FF, FontFamily: sqdoc.FontFamilySans}
}

func normalizeAttr(attr sqdoc.StyleAttr) sqdoc.StyleAttr {
	if attr.FontSizePt == 0 {
		attr.FontSizePt = 14
	}
	if attr.ColorRGBA == 0 {
		attr.ColorRGBA = 0x202020FF
	}
	if !isValidFontFamily(attr.FontFamily) {
		attr.FontFamily = sqdoc.FontFamilySans
	}
	return attr
}

func attrsEqual(a, b sqdoc.StyleAttr) bool {
	a = normalizeAttr(a)
	b = normalizeAttr(b)
	return a.Bold == b.Bold &&
		a.Italic == b.Italic &&
		a.Underline == b.Underline &&
		a.Highlight == b.Highlight &&
		a.FontFamily == b.FontFamily &&
		a.FontSizePt == b.FontSizePt &&
		a.ColorRGBA == b.ColorRGBA
}

func isValidFontFamily(f sqdoc.FontFamily) bool {
	return f == sqdoc.FontFamilySans || f == sqdoc.FontFamilySerif || f == sqdoc.FontFamilyMonospace
}

func sanitizeRuns(textLen int, runs []sqdoc.StyleRun) []sqdoc.StyleRun {
	if textLen < 0 {
		textLen = 0
	}
	if len(runs) == 0 {
		if textLen == 0 {
			return []sqdoc.StyleRun{{Start: 0, End: 0, Attr: defaultStyleAttr()}}
		}
		return []sqdoc.StyleRun{{Start: 0, End: uint32(textLen), Attr: defaultStyleAttr()}}
	}
	clean := make([]sqdoc.StyleRun, 0, len(runs))
	for _, r := range runs {
		start := int(r.Start)
		end := int(r.End)
		if start < 0 {
			start = 0
		}
		if end < 0 {
			end = 0
		}
		if start > textLen {
			start = textLen
		}
		if end > textLen {
			end = textLen
		}
		if start > end {
			start, end = end, start
		}
		if textLen > 0 && start == end {
			continue
		}
		if textLen == 0 && !(start == 0 && end == 0) {
			continue
		}
		clean = append(clean, sqdoc.StyleRun{Start: uint32(start), End: uint32(end), Attr: normalizeAttr(r.Attr)})
	}
	if len(clean) == 0 {
		if textLen == 0 {
			return []sqdoc.StyleRun{{Start: 0, End: 0, Attr: defaultStyleAttr()}}
		}
		return []sqdoc.StyleRun{{Start: 0, End: uint32(textLen), Attr: defaultStyleAttr()}}
	}

	sort.Slice(clean, func(i, j int) bool {
		if clean[i].Start == clean[j].Start {
			return clean[i].End < clean[j].End
		}
		return clean[i].Start < clean[j].Start
	})

	nonOverlap := make([]sqdoc.StyleRun, 0, len(clean))
	for _, r := range clean {
		if len(nonOverlap) == 0 {
			nonOverlap = append(nonOverlap, r)
			continue
		}
		last := &nonOverlap[len(nonOverlap)-1]
		if r.Start < last.End {
			if r.End <= last.End {
				continue
			}
			r.Start = last.End
		}
		if r.Start == r.End && textLen > 0 {
			continue
		}
		nonOverlap = append(nonOverlap, r)
	}

	merged := make([]sqdoc.StyleRun, 0, len(nonOverlap))
	for _, r := range nonOverlap {
		if len(merged) == 0 {
			merged = append(merged, r)
			continue
		}
		last := &merged[len(merged)-1]
		if last.End == r.Start && attrsEqual(last.Attr, r.Attr) {
			last.End = r.End
			continue
		}
		merged = append(merged, r)
	}

	if textLen == 0 {
		if len(merged) == 0 {
			return []sqdoc.StyleRun{{Start: 0, End: 0, Attr: defaultStyleAttr()}}
		}
		return []sqdoc.StyleRun{{Start: 0, End: 0, Attr: normalizeAttr(merged[0].Attr)}}
	}

	if len(merged) == 0 {
		return []sqdoc.StyleRun{{Start: 0, End: uint32(textLen), Attr: defaultStyleAttr()}}
	}
	if merged[0].Start > 0 {
		merged = append([]sqdoc.StyleRun{{Start: 0, End: merged[0].Start, Attr: defaultStyleAttr()}}, merged...)
	}
	if int(merged[len(merged)-1].End) < textLen {
		merged = append(merged, sqdoc.StyleRun{Start: merged[len(merged)-1].End, End: uint32(textLen), Attr: defaultStyleAttr()})
	}
	for i := 1; i < len(merged); i++ {
		if merged[i].Start > merged[i-1].End {
			gap := sqdoc.StyleRun{Start: merged[i-1].End, End: merged[i].Start, Attr: defaultStyleAttr()}
			merged = append(merged[:i], append([]sqdoc.StyleRun{gap}, merged[i:]...)...)
			i++
		}
	}
	return merged
}

func coverageRuns(textLen int, runs []sqdoc.StyleRun) []sqdoc.StyleRun {
	return sanitizeRuns(textLen, runs)
}

func normalizeSparseRuns(runs []sqdoc.StyleRun) []sqdoc.StyleRun {
	if len(runs) == 0 {
		return nil
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Start == runs[j].Start {
			return runs[i].End < runs[j].End
		}
		return runs[i].Start < runs[j].Start
	})
	out := make([]sqdoc.StyleRun, 0, len(runs))
	for _, r := range runs {
		if r.Start >= r.End {
			continue
		}
		r.Attr = normalizeAttr(r.Attr)
		if len(out) == 0 {
			out = append(out, r)
			continue
		}
		last := &out[len(out)-1]
		if r.Start < last.End {
			if r.End <= last.End {
				continue
			}
			r.Start = last.End
		}
		if last.End == r.Start && attrsEqual(last.Attr, r.Attr) {
			last.End = r.End
			continue
		}
		out = append(out, r)
	}
	return out
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
