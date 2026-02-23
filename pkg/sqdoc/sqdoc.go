package sqdoc

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/pbkdf2"
)

const (
	MagicString      = "KeepCalmAndFuckTheRussians"
	VersionV1        = uint16(1)
	FlagRandomAccess = uint16(1 << 0)

	headerSize  = 42
	tocEntSize  = 8 + 1 + 8 + 4 + 4
	styleEntSz  = 8 + 4 + 4 + 1 + 1 + 2 + 4
	styleEntV1  = 8 + 4 + 4 + 1 + 2 + 4
	metaBlockID = uint64(0)
	fmtBlockID  = ^uint64(0)

	secureMagic      = "SQDOC_FUCK_THE_RUSSIANS"
	secureVersionV1  = uint16(1)
	secureFlagComp   = uint16(1 << 0)
	secureFlagEnc    = uint16(1 << 1)
	secureSaltSize   = 16
	secureNonceSize  = 12
	secureHeaderSize = len(secureMagic) + 2 + 2 + secureSaltSize + secureNonceSize + 8
	kdfIterations    = 200000
)

type EncryptionOptions struct {
	Enabled  bool
	Password string
}

type SaveOptions struct {
	Compression bool
	Encryption  EncryptionOptions
}

type LoadOptions struct {
	Password string
}

type EnvelopeInfo struct {
	Wrapped     bool
	Compressed  bool
	Encrypted   bool
	EnvelopeVer uint16
}

type BlockKind uint8

const (
	BlockKindMetadata BlockKind = 0
	BlockKindText     BlockKind = 1
	BlockKindMedia    BlockKind = 2
	BlockKindStyle    BlockKind = 3
	BlockKindScript   BlockKind = 4
)

type Document struct {
	Metadata Metadata
	Blocks   []Block
}

type FontFamily uint8

const (
	FontFamilySans FontFamily = iota
	FontFamilySerif
	FontFamilyMonospace
)

type Metadata struct {
	Author              string
	Title               string
	CreatedUnix         int64
	ModifiedUnix        int64
	PagedMode           bool
	ParagraphGap        uint16
	PreferredFontFamily FontFamily
}

type Block struct {
	ID   uint64
	Kind BlockKind
	Text *TextBlock
}

type TextBlock struct {
	UTF8 []byte
	Runs []StyleRun
}

type StyleRun struct {
	Start uint32
	End   uint32
	Attr  StyleAttr
}

type StyleAttr struct {
	Bold       bool
	Italic     bool
	Underline  bool
	Highlight  bool
	FontFamily FontFamily
	FontSizePt uint16
	ColorRGBA  uint32
}

type FormattingDirectiveEntry struct {
	BlockID uint64
	Start   uint32
	End     uint32
	Attr    StyleAttr
}

type LayoutSegment struct {
	Name    string
	Kind    BlockKind
	BlockID uint64
	Offset  uint64
	Length  uint32
}

type LayoutInfo struct {
	HeaderLength uint32
	IndexOffset  uint64
	IndexLength  uint32
	FileSize     uint64
	Segments     []LayoutSegment
}

type tocEntry struct {
	ID     uint64
	Kind   BlockKind
	Offset uint64
	Length uint32
	CRC32  uint32
}

type encodeResult struct {
	Blob      []byte
	Entries   []tocEntry
	TOCOffset uint64
	TOCLength uint32
}

type payloadEntry struct {
	ID      uint64
	Kind    BlockKind
	Payload []byte
}

var (
	ErrInvalidMagic      = errors.New("sqdoc: invalid magic")
	ErrUnsupportedVer    = errors.New("sqdoc: unsupported version")
	ErrMissingRandomFlag = errors.New("sqdoc: random-access flag required")
	ErrInvalidTOC        = errors.New("sqdoc: invalid toc")
	ErrInvalidBlockRange = errors.New("sqdoc: invalid block range")
	ErrOverlappingBlocks = errors.New("sqdoc: overlapping block ranges")
	ErrPasswordRequired  = errors.New("sqdoc: password required")
	ErrInvalidPassword   = errors.New("sqdoc: invalid password")
	ErrInvalidSecureFile = errors.New("sqdoc: invalid secure file")
)

func NewDocument(author, title string) *Document {
	now := time.Now().Unix()
	return &Document{Metadata: Metadata{
		Author:              author,
		Title:               title,
		CreatedUnix:         now,
		ModifiedUnix:        now,
		PagedMode:           false,
		ParagraphGap:        8,
		PreferredFontFamily: FontFamilySans,
	}}
}

func CloneDocument(doc *Document) *Document {
	if doc == nil {
		return nil
	}
	out := &Document{Metadata: doc.Metadata, Blocks: make([]Block, len(doc.Blocks))}
	for i, b := range doc.Blocks {
		out.Blocks[i] = Block{ID: b.ID, Kind: b.Kind}
		if b.Text != nil {
			tb := &TextBlock{UTF8: append([]byte(nil), b.Text.UTF8...), Runs: make([]StyleRun, len(b.Text.Runs))}
			copy(tb.Runs, b.Text.Runs)
			out.Blocks[i].Text = tb
		}
	}
	return out
}

func Save(path string, doc *Document) error {
	return SaveWithOptions(path, doc, SaveOptions{})
}

func SaveWithOptions(path string, doc *Document, opts SaveOptions) error {
	if doc == nil {
		return errors.New("sqdoc: document is nil")
	}
	now := time.Now().Unix()
	if doc.Metadata.CreatedUnix == 0 {
		doc.Metadata.CreatedUnix = now
	}
	doc.Metadata.ModifiedUnix = now

	if err := Validate(doc); err != nil {
		return err
	}

	res, err := encodeDocumentDetailed(doc)
	if err != nil {
		return err
	}
	blob := res.Blob

	if opts.Compression {
		blob, err = compressBytes(blob)
		if err != nil {
			return err
		}
	}

	if opts.Encryption.Enabled {
		if stringsTrim(opts.Encryption.Password) == "" {
			return ErrPasswordRequired
		}
	}

	if opts.Compression || opts.Encryption.Enabled {
		blob, err = encodeSecureEnvelope(blob, opts)
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Load(path string) (*Document, error) {
	return LoadWithOptions(path, LoadOptions{})
}

func LoadWithOptions(path string, opts LoadOptions) (*Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isSecureEnvelope(b) {
		b, err = decodeSecureEnvelope(b, opts)
		if err != nil {
			return nil, err
		}
	}
	doc, err := decodeDocument(b)
	if err != nil {
		return nil, err
	}
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func InspectEnvelope(path string) (EnvelopeInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return EnvelopeInfo{}, err
	}
	return inspectEnvelopeBytes(b)
}

func InspectLayout(doc *Document) (*LayoutInfo, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}
	res, err := encodeDocumentDetailed(doc)
	if err != nil {
		return nil, err
	}

	segments := []LayoutSegment{{
		Name:    "Header",
		Kind:    BlockKindMetadata,
		BlockID: metaBlockID,
		Offset:  0,
		Length:  headerSize,
	}, {
		Name:    "Index",
		Kind:    BlockKindStyle,
		BlockID: fmtBlockID,
		Offset:  res.TOCOffset,
		Length:  res.TOCLength,
	}}
	for _, e := range res.Entries {
		name := "Block"
		switch e.Kind {
		case BlockKindMetadata:
			name = "Metadata"
		case BlockKindStyle:
			name = "Formatting Directive"
		case BlockKindText:
			name = "Data Block"
		}
		segments = append(segments, LayoutSegment{
			Name:    name,
			Kind:    e.Kind,
			BlockID: e.ID,
			Offset:  e.Offset,
			Length:  e.Length,
		})
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].Offset < segments[j].Offset })

	return &LayoutInfo{
		HeaderLength: headerSize,
		IndexOffset:  res.TOCOffset,
		IndexLength:  res.TOCLength,
		FileSize:     uint64(len(res.Blob)),
		Segments:     segments,
	}, nil
}

func Validate(doc *Document) error {
	if doc == nil {
		return errors.New("sqdoc: document is nil")
	}
	if !utf8.ValidString(doc.Metadata.Author) || !utf8.ValidString(doc.Metadata.Title) {
		return errors.New("sqdoc: metadata fields must be valid UTF-8")
	}
	if !isValidFontFamily(doc.Metadata.PreferredFontFamily) {
		return errors.New("sqdoc: metadata preferred font family is invalid")
	}

	seenIDs := map[uint64]struct{}{}
	for i := range doc.Blocks {
		b := &doc.Blocks[i]
		if b.ID == 0 || b.ID == fmtBlockID {
			return fmt.Errorf("sqdoc: block[%d] id is reserved", i)
		}
		if _, ok := seenIDs[b.ID]; ok {
			return fmt.Errorf("sqdoc: duplicate block id %d", b.ID)
		}
		seenIDs[b.ID] = struct{}{}

		if b.Kind != BlockKindText {
			return fmt.Errorf("sqdoc: unsupported block kind %d for save", b.Kind)
		}
		if b.Text == nil {
			return fmt.Errorf("sqdoc: text block %d missing payload", b.ID)
		}
		if !utf8.Valid(b.Text.UTF8) {
			return fmt.Errorf("sqdoc: text block %d is not valid UTF-8", b.ID)
		}
		if err := validateRuns(b.Text); err != nil {
			return fmt.Errorf("sqdoc: block %d: %w", b.ID, err)
		}
	}
	return nil
}

func validateRuns(tb *TextBlock) error {
	txtLen := uint32(len(tb.UTF8))
	runs := append([]StyleRun(nil), tb.Runs...)
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Start == runs[j].Start {
			return runs[i].End < runs[j].End
		}
		return runs[i].Start < runs[j].Start
	})

	var lastEnd uint32
	for i, r := range runs {
		if r.Start > r.End {
			return fmt.Errorf("invalid run range %d..%d", r.Start, r.End)
		}
		if r.Start == r.End {
			if !(txtLen == 0 && r.Start == 0) {
				return fmt.Errorf("invalid zero-length run %d..%d", r.Start, r.End)
			}
		}
		if r.End > txtLen {
			return fmt.Errorf("run range %d..%d outside text length %d", r.Start, r.End, txtLen)
		}
		if i > 0 && r.Start < lastEnd {
			return fmt.Errorf("overlapping style runs around offset %d", r.Start)
		}
		if r.Attr.FontSizePt == 0 {
			return errors.New("font size must be non-zero")
		}
		if !isValidFontFamily(r.Attr.FontFamily) {
			return errors.New("font family is invalid")
		}
		lastEnd = r.End
	}
	return nil
}

func encodeDocument(doc *Document) ([]byte, error) {
	res, err := encodeDocumentDetailed(doc)
	if err != nil {
		return nil, err
	}
	return res.Blob, nil
}

func encodeDocumentDetailed(doc *Document) (*encodeResult, error) {
	payloads := make([]payloadEntry, 0, len(doc.Blocks)+2)

	metaPayload := encodeMetadata(doc.Metadata)
	payloads = append(payloads, payloadEntry{
		ID:      metaBlockID,
		Kind:    BlockKindMetadata,
		Payload: metaPayload,
	})

	fmtPayload := encodeFormattingDirective(collectFormatting(doc))
	payloads = append(payloads, payloadEntry{
		ID:      fmtBlockID,
		Kind:    BlockKindStyle,
		Payload: fmtPayload,
	})

	for _, b := range doc.Blocks {
		payload, err := encodeBlockPayload(b)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payloadEntry{
			ID:      b.ID,
			Kind:    b.Kind,
			Payload: payload,
		})
	}

	tocOffset := uint64(headerSize)
	tocLength := uint32(len(payloads) * tocEntSize)
	out := make([]byte, headerSize+int(tocLength))
	copy(out[:26], []byte(MagicString))

	entries := make([]tocEntry, 0, len(payloads))
	offset := uint64(len(out))
	for _, p := range payloads {
		entries = append(entries, tocEntry{
			ID:     p.ID,
			Kind:   p.Kind,
			Offset: offset,
			Length: uint32(len(p.Payload)),
			CRC32:  crc32.ChecksumIEEE(p.Payload),
		})
		out = append(out, p.Payload...)
		offset += uint64(len(p.Payload))
	}

	ptr := headerSize
	for _, e := range entries {
		binary.LittleEndian.PutUint64(out[ptr:ptr+8], e.ID)
		out[ptr+8] = byte(e.Kind)
		binary.LittleEndian.PutUint64(out[ptr+9:ptr+17], e.Offset)
		binary.LittleEndian.PutUint32(out[ptr+17:ptr+21], e.Length)
		binary.LittleEndian.PutUint32(out[ptr+21:ptr+25], e.CRC32)
		ptr += tocEntSize
	}

	binary.LittleEndian.PutUint16(out[26:28], VersionV1)
	binary.LittleEndian.PutUint16(out[28:30], FlagRandomAccess)
	binary.LittleEndian.PutUint64(out[30:38], tocOffset)
	binary.LittleEndian.PutUint32(out[38:42], uint32(len(entries)))

	return &encodeResult{Blob: out, Entries: entries, TOCOffset: tocOffset, TOCLength: tocLength}, nil
}

func decodeDocument(blob []byte) (*Document, error) {
	if len(blob) < headerSize {
		return nil, ErrInvalidMagic
	}
	if string(blob[:26]) != MagicString {
		return nil, ErrInvalidMagic
	}
	if v := binary.LittleEndian.Uint16(blob[26:28]); v != VersionV1 {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVer, v)
	}
	if flags := binary.LittleEndian.Uint16(blob[28:30]); flags&FlagRandomAccess == 0 {
		return nil, ErrMissingRandomFlag
	}

	tocOffset := binary.LittleEndian.Uint64(blob[30:38])
	tocCount := binary.LittleEndian.Uint32(blob[38:42])
	if tocOffset > uint64(len(blob)) {
		return nil, ErrInvalidTOC
	}
	end := tocOffset + uint64(tocCount)*uint64(tocEntSize)
	if end > uint64(len(blob)) {
		return nil, ErrInvalidTOC
	}

	entries := make([]tocEntry, 0, tocCount)
	ptr := int(tocOffset)
	for i := 0; i < int(tocCount); i++ {
		entries = append(entries, tocEntry{
			ID:     binary.LittleEndian.Uint64(blob[ptr : ptr+8]),
			Kind:   BlockKind(blob[ptr+8]),
			Offset: binary.LittleEndian.Uint64(blob[ptr+9 : ptr+17]),
			Length: binary.LittleEndian.Uint32(blob[ptr+17 : ptr+21]),
			CRC32:  binary.LittleEndian.Uint32(blob[ptr+21 : ptr+25]),
		})
		ptr += tocEntSize
	}

	if err := validateEntryRanges(entries, len(blob)); err != nil {
		return nil, err
	}

	doc := &Document{}
	blockByID := map[uint64]*Block{}
	var directive []FormattingDirectiveEntry

	for _, e := range entries {
		start := int(e.Offset)
		stop := start + int(e.Length)
		payload := blob[start:stop]
		if crc32.ChecksumIEEE(payload) != e.CRC32 {
			return nil, fmt.Errorf("sqdoc: crc mismatch for block %d", e.ID)
		}

		switch e.Kind {
		case BlockKindMetadata:
			m, err := decodeMetadata(payload)
			if err != nil {
				return nil, err
			}
			doc.Metadata = m
		case BlockKindStyle:
			fmtEntries, err := decodeFormattingDirective(payload)
			if err != nil {
				return nil, err
			}
			directive = fmtEntries
		case BlockKindText:
			tb, err := decodeTextBlock(payload)
			if err != nil {
				return nil, err
			}
			blk := Block{ID: e.ID, Kind: BlockKindText, Text: tb}
			doc.Blocks = append(doc.Blocks, blk)
			blockByID[e.ID] = &doc.Blocks[len(doc.Blocks)-1]
		default:
			// Forward compatible: unknown kinds remain skippable via TOC.
		}
	}

	if len(directive) > 0 {
		for _, d := range directive {
			if b := blockByID[d.BlockID]; b != nil && b.Text != nil {
				b.Text.Runs = append(b.Text.Runs, StyleRun{Start: d.Start, End: d.End, Attr: d.Attr})
			}
		}
	}
	for i := range doc.Blocks {
		tb := doc.Blocks[i].Text
		if tb == nil {
			continue
		}
		sort.Slice(tb.Runs, func(a, b int) bool {
			if tb.Runs[a].Start == tb.Runs[b].Start {
				return tb.Runs[a].End < tb.Runs[b].End
			}
			return tb.Runs[a].Start < tb.Runs[b].Start
		})
	}

	return doc, nil
}

func collectFormatting(doc *Document) []FormattingDirectiveEntry {
	out := make([]FormattingDirectiveEntry, 0)
	for _, b := range doc.Blocks {
		if b.Kind != BlockKindText || b.Text == nil {
			continue
		}
		for _, r := range b.Text.Runs {
			out = append(out, FormattingDirectiveEntry{
				BlockID: b.ID,
				Start:   r.Start,
				End:     r.End,
				Attr:    r.Attr,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BlockID == out[j].BlockID {
			if out[i].Start == out[j].Start {
				return out[i].End < out[j].End
			}
			return out[i].Start < out[j].Start
		}
		return out[i].BlockID < out[j].BlockID
	})
	return out
}

func encodeFormattingDirective(entries []FormattingDirectiveEntry) []byte {
	out := make([]byte, 0, 4+len(entries)*styleEntSz)
	out = appendU32(out, uint32(len(entries)))
	for _, e := range entries {
		out = appendU64(out, e.BlockID)
		out = appendU32(out, e.Start)
		out = appendU32(out, e.End)
		flags := uint8(0)
		if e.Attr.Bold {
			flags |= 1
		}
		if e.Attr.Italic {
			flags |= 2
		}
		if e.Attr.Underline {
			flags |= 4
		}
		if e.Attr.Highlight {
			flags |= 8
		}
		out = append(out, flags)
		out = append(out, byte(normalizeFontFamily(e.Attr.FontFamily)))
		out = appendU16(out, e.Attr.FontSizePt)
		out = appendU32(out, e.Attr.ColorRGBA)
	}
	return out
}

func decodeFormattingDirective(b []byte) ([]FormattingDirectiveEntry, error) {
	if len(b) < 4 {
		return nil, errors.New("sqdoc: malformed formatting directive block")
	}
	count := int(binary.LittleEndian.Uint32(b[:4]))
	ptr := 4
	out := make([]FormattingDirectiveEntry, 0, count)
	if count == 0 {
		return out, nil
	}
	remaining := len(b) - 4
	if remaining < 0 || remaining%count != 0 {
		return nil, errors.New("sqdoc: malformed formatting directive entry length")
	}
	entrySize := remaining / count
	if entrySize != styleEntSz && entrySize != styleEntV1 {
		return nil, errors.New("sqdoc: unsupported formatting directive entry size")
	}
	for i := 0; i < count; i++ {
		if len(b[ptr:]) < entrySize {
			return nil, errors.New("sqdoc: malformed formatting directive entry")
		}
		blockID := binary.LittleEndian.Uint64(b[ptr : ptr+8])
		start := binary.LittleEndian.Uint32(b[ptr+8 : ptr+12])
		end := binary.LittleEndian.Uint32(b[ptr+12 : ptr+16])
		flags := b[ptr+16]
		fontFamily := FontFamilySans
		fontOffset := 17
		if entrySize == styleEntSz {
			fontFamily = normalizeFontFamily(FontFamily(b[ptr+17]))
			fontOffset = 18
		}
		fontSize := binary.LittleEndian.Uint16(b[ptr+fontOffset : ptr+fontOffset+2])
		color := binary.LittleEndian.Uint32(b[ptr+fontOffset+2 : ptr+fontOffset+6])
		ptr += entrySize
		out = append(out, FormattingDirectiveEntry{
			BlockID: blockID,
			Start:   start,
			End:     end,
			Attr: StyleAttr{
				Bold:       flags&1 != 0,
				Italic:     flags&2 != 0,
				Underline:  flags&4 != 0,
				Highlight:  flags&8 != 0,
				FontFamily: fontFamily,
				FontSizePt: fontSize,
				ColorRGBA:  color,
			},
		})
	}
	return out, nil
}

func validateEntryRanges(entries []tocEntry, fileLen int) error {
	type rng struct{ start, end uint64 }
	ranges := make([]rng, 0, len(entries))

	for _, e := range entries {
		if e.Offset > uint64(fileLen) {
			return ErrInvalidBlockRange
		}
		end := e.Offset + uint64(e.Length)
		if end > uint64(fileLen) {
			return ErrInvalidBlockRange
		}
		ranges = append(ranges, rng{start: e.Offset, end: end})
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start < ranges[i-1].end {
			return ErrOverlappingBlocks
		}
	}
	return nil
}

func encodeMetadata(m Metadata) []byte {
	out := make([]byte, 0, 64)
	out = appendString(out, m.Author)
	out = appendString(out, m.Title)
	out = appendI64(out, m.CreatedUnix)
	out = appendI64(out, m.ModifiedUnix)
	flags := byte(0)
	if m.PagedMode {
		flags |= 1
	}
	out = append(out, flags)
	out = appendU16(out, m.ParagraphGap)
	out = append(out, byte(normalizeFontFamily(m.PreferredFontFamily)))
	return out
}

func decodeMetadata(b []byte) (Metadata, error) {
	var m Metadata
	var ok bool
	if m.Author, b, ok = readString(b); !ok {
		return m, errors.New("sqdoc: malformed metadata author")
	}
	if m.Title, b, ok = readString(b); !ok {
		return m, errors.New("sqdoc: malformed metadata title")
	}
	if len(b) < 16 {
		return m, errors.New("sqdoc: malformed metadata timestamps")
	}
	m.CreatedUnix = int64(binary.LittleEndian.Uint64(b[:8]))
	m.ModifiedUnix = int64(binary.LittleEndian.Uint64(b[8:16]))
	b = b[16:]
	if len(b) == 0 {
		m.PagedMode = false
		m.ParagraphGap = 8
		m.PreferredFontFamily = FontFamilySans
		return m, nil
	}
	if len(b) < 4 {
		return m, errors.New("sqdoc: malformed metadata settings")
	}
	flags := b[0]
	m.PagedMode = flags&1 != 0
	m.ParagraphGap = binary.LittleEndian.Uint16(b[1:3])
	m.PreferredFontFamily = normalizeFontFamily(FontFamily(b[3]))
	return m, nil
}

func encodeBlockPayload(b Block) ([]byte, error) {
	switch b.Kind {
	case BlockKindText:
		if b.Text == nil {
			return nil, errors.New("sqdoc: text block payload is nil")
		}
		return encodeTextBlock(b.Text), nil
	default:
		return nil, fmt.Errorf("sqdoc: unsupported block kind %d", b.Kind)
	}
}

func encodeTextBlock(tb *TextBlock) []byte {
	out := make([]byte, 0, len(tb.UTF8)+4)
	out = appendU32(out, uint32(len(tb.UTF8)))
	out = append(out, tb.UTF8...)
	return out
}

func decodeTextBlock(b []byte) (*TextBlock, error) {
	if len(b) < 4 {
		return nil, errors.New("sqdoc: malformed text block")
	}
	txtLen := int(binary.LittleEndian.Uint32(b[:4]))
	if len(b) < 4+txtLen {
		return nil, errors.New("sqdoc: malformed text payload")
	}
	txt := append([]byte(nil), b[4:4+txtLen]...)
	ptr := 4 + txtLen

	tb := &TextBlock{UTF8: txt}
	if len(b[ptr:]) < 4 {
		return tb, nil
	}

	// Backward-compatibility parser for older experimental payloads that encoded
	// style runs directly in the text block.
	runCount := int(binary.LittleEndian.Uint32(b[ptr : ptr+4]))
	ptr += 4
	runs := make([]StyleRun, 0, runCount)
	for i := 0; i < runCount; i++ {
		if len(b[ptr:]) < 4+4+1+2+4 {
			return tb, nil
		}
		start := binary.LittleEndian.Uint32(b[ptr : ptr+4])
		end := binary.LittleEndian.Uint32(b[ptr+4 : ptr+8])
		flags := b[ptr+8]
		font := binary.LittleEndian.Uint16(b[ptr+9 : ptr+11])
		color := binary.LittleEndian.Uint32(b[ptr+11 : ptr+15])
		ptr += 15

		runs = append(runs, StyleRun{
			Start: start,
			End:   end,
			Attr: StyleAttr{
				Bold:       flags&1 != 0,
				Italic:     flags&2 != 0,
				Underline:  flags&4 != 0,
				Highlight:  flags&8 != 0,
				FontFamily: FontFamilySans,
				FontSizePt: font,
				ColorRGBA:  color,
			},
		})
	}
	tb.Runs = runs
	return tb, nil
}

func appendString(dst []byte, s string) []byte {
	dst = appendU32(dst, uint32(len(s)))
	return append(dst, s...)
}

func readString(src []byte) (string, []byte, bool) {
	if len(src) < 4 {
		return "", nil, false
	}
	ln := int(binary.LittleEndian.Uint32(src[:4]))
	src = src[4:]
	if len(src) < ln {
		return "", nil, false
	}
	return string(src[:ln]), src[ln:], true
}

func appendU16(dst []byte, v uint16) []byte {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	return append(dst, b[:]...)
}

func appendU32(dst []byte, v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return append(dst, b[:]...)
}

func appendU64(dst []byte, v uint64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	return append(dst, b[:]...)
}

func appendI64(dst []byte, v int64) []byte {
	return appendU64(dst, uint64(v))
}

func stringsTrim(s string) string {
	i := 0
	j := len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func isValidFontFamily(f FontFamily) bool {
	return f == FontFamilySans || f == FontFamilySerif || f == FontFamilyMonospace
}

func normalizeFontFamily(f FontFamily) FontFamily {
	if !isValidFontFamily(f) {
		return FontFamilySans
	}
	return f
}

func isSecureEnvelope(b []byte) bool {
	return len(b) >= len(secureMagic) && string(b[:len(secureMagic)]) == secureMagic
}

func inspectEnvelopeBytes(b []byte) (EnvelopeInfo, error) {
	info := EnvelopeInfo{}
	if !isSecureEnvelope(b) {
		return info, nil
	}
	if len(b) < secureHeaderSize {
		return info, ErrInvalidSecureFile
	}
	version := binary.LittleEndian.Uint16(b[len(secureMagic) : len(secureMagic)+2])
	if version != secureVersionV1 {
		return info, fmt.Errorf("%w: secure envelope version %d", ErrUnsupportedVer, version)
	}
	flags := binary.LittleEndian.Uint16(b[len(secureMagic)+2 : len(secureMagic)+4])
	info.Wrapped = true
	info.Compressed = flags&secureFlagComp != 0
	info.Encrypted = flags&secureFlagEnc != 0
	info.EnvelopeVer = version
	return info, nil
}

func encodeSecureEnvelope(payload []byte, opts SaveOptions) ([]byte, error) {
	flags := uint16(0)
	if opts.Compression {
		flags |= secureFlagComp
	}
	if opts.Encryption.Enabled {
		flags |= secureFlagEnc
	}

	salt := make([]byte, secureSaltSize)
	nonce := make([]byte, secureNonceSize)
	if opts.Encryption.Enabled {
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return nil, err
		}

		key := pbkdf2.Key([]byte(opts.Encryption.Password), salt, kdfIterations, 32, sha256.New)
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		payload = gcm.Seal(nil, nonce, payload, nil)
	}

	out := make([]byte, secureHeaderSize)
	copy(out[:len(secureMagic)], []byte(secureMagic))
	binary.LittleEndian.PutUint16(out[len(secureMagic):len(secureMagic)+2], secureVersionV1)
	binary.LittleEndian.PutUint16(out[len(secureMagic)+2:len(secureMagic)+4], flags)
	copy(out[len(secureMagic)+4:len(secureMagic)+4+secureSaltSize], salt)
	copy(out[len(secureMagic)+4+secureSaltSize:len(secureMagic)+4+secureSaltSize+secureNonceSize], nonce)
	binary.LittleEndian.PutUint64(out[len(secureMagic)+4+secureSaltSize+secureNonceSize:], uint64(len(payload)))
	out = append(out, payload...)
	return out, nil
}

func decodeSecureEnvelope(b []byte, opts LoadOptions) ([]byte, error) {
	info, err := inspectEnvelopeBytes(b)
	if err != nil {
		return nil, err
	}
	if !info.Wrapped {
		return nil, ErrInvalidSecureFile
	}
	flags := binary.LittleEndian.Uint16(b[len(secureMagic)+2 : len(secureMagic)+4])
	salt := append([]byte(nil), b[len(secureMagic)+4:len(secureMagic)+4+secureSaltSize]...)
	nonce := append([]byte(nil), b[len(secureMagic)+4+secureSaltSize:len(secureMagic)+4+secureSaltSize+secureNonceSize]...)
	payloadLen := binary.LittleEndian.Uint64(b[len(secureMagic)+4+secureSaltSize+secureNonceSize:])
	if uint64(len(b)-secureHeaderSize) != payloadLen {
		return nil, ErrInvalidSecureFile
	}
	payload := append([]byte(nil), b[secureHeaderSize:]...)

	if flags&secureFlagEnc != 0 {
		if stringsTrim(opts.Password) == "" {
			return nil, ErrPasswordRequired
		}
		key := pbkdf2.Key([]byte(opts.Password), salt, kdfIterations, 32, sha256.New)
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		payload, err = gcm.Open(nil, nonce, payload, nil)
		if err != nil {
			return nil, ErrInvalidPassword
		}
	}

	if flags&secureFlagComp != 0 {
		var err error
		payload, err = decompressBytes(payload)
		if err != nil {
			return nil, err
		}
	}

	return payload, nil
}

func compressBytes(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(in); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressBytes(in []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
