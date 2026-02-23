package sqdoc

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripSaveLoad(t *testing.T) {
	doc := NewDocument("Alex", "Draft")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   1,
		Kind: BlockKindText,
		Text: &TextBlock{
			UTF8: []byte("Hello SQDoc"),
			Runs: []StyleRun{{
				Start: 0,
				End:   5,
				Attr:  StyleAttr{Bold: true, Highlight: true, FontSizePt: 12, ColorRGBA: 0x112233FF},
			}},
		},
	})

	path := filepath.Join(t.TempDir(), "roundtrip.sqdoc")
	if err := Save(path, doc); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Metadata.Title != doc.Metadata.Title {
		t.Fatalf("title mismatch: got %q want %q", loaded.Metadata.Title, doc.Metadata.Title)
	}
	if len(loaded.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(loaded.Blocks))
	}
	if string(loaded.Blocks[0].Text.UTF8) != "Hello SQDoc" {
		t.Fatalf("unexpected text payload: %q", string(loaded.Blocks[0].Text.UTF8))
	}
	if len(loaded.Blocks[0].Text.Runs) != 1 || !loaded.Blocks[0].Text.Runs[0].Attr.Bold || !loaded.Blocks[0].Text.Runs[0].Attr.Highlight {
		t.Fatalf("style run mismatch: %#v", loaded.Blocks[0].Text.Runs)
	}
}

func TestLoadRejectsBadMagic(t *testing.T) {
	blob := make([]byte, headerSize)
	copy(blob[:26], []byte("badbadbadbadbadbadbadbadba"))
	binary.LittleEndian.PutUint16(blob[26:28], VersionV1)

	path := filepath.Join(t.TempDir(), "badmagic.sqdoc")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if !errors.Is(err, ErrInvalidMagic) {
		t.Fatalf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestValidateRejectsOverlappingRuns(t *testing.T) {
	doc := NewDocument("", "")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   10,
		Kind: BlockKindText,
		Text: &TextBlock{
			UTF8: []byte("abcdef"),
			Runs: []StyleRun{
				{Start: 0, End: 4, Attr: StyleAttr{FontSizePt: 12}},
				{Start: 3, End: 6, Attr: StyleAttr{FontSizePt: 12}},
			},
		},
	})
	if err := Validate(doc); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateAllowsEmptyBlockStyleRun(t *testing.T) {
	doc := NewDocument("", "")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   11,
		Kind: BlockKindText,
		Text: &TextBlock{
			UTF8: []byte{},
			Runs: []StyleRun{{Start: 0, End: 0, Attr: StyleAttr{FontSizePt: 12}}},
		},
	})
	if err := Validate(doc); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
}

func TestValidateRejectsZeroLengthRunOnNonEmptyText(t *testing.T) {
	doc := NewDocument("", "")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   12,
		Kind: BlockKindText,
		Text: &TextBlock{
			UTF8: []byte("abc"),
			Runs: []StyleRun{{Start: 1, End: 1, Attr: StyleAttr{FontSizePt: 12}}},
		},
	})
	if err := Validate(doc); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestLoadRejectsMissingRandomAccessFlag(t *testing.T) {
	doc := NewDocument("", "flags")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   1,
		Kind: BlockKindText,
		Text: &TextBlock{
			UTF8: []byte("abc"),
			Runs: []StyleRun{{Start: 0, End: 3, Attr: StyleAttr{FontSizePt: 12}}},
		},
	})
	blob, err := encodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint16(blob[28:30], 0)

	path := filepath.Join(t.TempDir(), "noflag.sqdoc")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(path)
	if !errors.Is(err, ErrMissingRandomFlag) {
		t.Fatalf("expected ErrMissingRandomFlag, got %v", err)
	}
}

func TestLoadRejectsOverlappingTOCRanges(t *testing.T) {
	doc := NewDocument("", "toc")
	doc.Blocks = append(doc.Blocks, Block{ID: 1, Kind: BlockKindText, Text: &TextBlock{UTF8: []byte("A"), Runs: []StyleRun{{Start: 0, End: 1, Attr: StyleAttr{FontSizePt: 12}}}}})
	blob, err := encodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	tocOffset := int(binary.LittleEndian.Uint64(blob[30:38]))
	if tocOffset <= 0 {
		t.Fatalf("invalid toc offset")
	}

	// Force metadata and first text block to overlap by setting text offset to metadata offset.
	metaOffset := binary.LittleEndian.Uint64(blob[tocOffset+9 : tocOffset+17])
	secondEntryOffsetField := tocOffset + tocEntSize + 9
	binary.LittleEndian.PutUint64(blob[secondEntryOffsetField:secondEntryOffsetField+8], metaOffset)

	path := filepath.Join(t.TempDir(), "overlap.sqdoc")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(path)
	if !errors.Is(err, ErrOverlappingBlocks) {
		t.Fatalf("expected ErrOverlappingBlocks, got %v", err)
	}
}

func TestInspectLayoutIncludesFormattingAndIndex(t *testing.T) {
	doc := NewDocument("", "layout")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   7,
		Kind: BlockKindText,
		Text: &TextBlock{
			UTF8: []byte("abc"),
			Runs: []StyleRun{{Start: 0, End: 3, Attr: StyleAttr{Bold: true, FontSizePt: 12}}},
		},
	})

	info, err := InspectLayout(doc)
	if err != nil {
		t.Fatalf("inspect layout failed: %v", err)
	}
	if info.HeaderLength != headerSize {
		t.Fatalf("header length mismatch: got %d want %d", info.HeaderLength, headerSize)
	}

	var sawStyle, sawIndex bool
	for _, seg := range info.Segments {
		if seg.Kind == BlockKindStyle && seg.Name == "Formatting Directive" {
			sawStyle = true
		}
		if seg.Name == "Index" {
			sawIndex = true
		}
	}
	if !sawStyle {
		t.Fatalf("expected formatting directive segment")
	}
	if !sawIndex {
		t.Fatalf("expected index segment")
	}
}

func TestIndexIsAtStartAndDataBlocksAfterDirective(t *testing.T) {
	doc := NewDocument("a", "b")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   1,
		Kind: BlockKindText,
		Text: &TextBlock{UTF8: []byte("one"), Runs: []StyleRun{{Start: 0, End: 3, Attr: StyleAttr{FontSizePt: 12}}}},
	})
	doc.Blocks = append(doc.Blocks, Block{
		ID:   2,
		Kind: BlockKindText,
		Text: &TextBlock{UTF8: []byte("two"), Runs: []StyleRun{{Start: 0, End: 3, Attr: StyleAttr{FontSizePt: 12}}}},
	})

	blob, err := encodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	tocOffset := binary.LittleEndian.Uint64(blob[30:38])
	tocCount := binary.LittleEndian.Uint32(blob[38:42])
	if tocOffset != headerSize {
		t.Fatalf("expected toc at %d, got %d", headerSize, tocOffset)
	}
	if tocCount != 4 { // metadata + formatting + 2 data blocks
		t.Fatalf("expected 4 toc entries, got %d", tocCount)
	}

	ptr := int(tocOffset)
	var lastDataOffset uint64
	var dataCount int
	for i := 0; i < int(tocCount); i++ {
		kind := BlockKind(blob[ptr+8])
		offset := binary.LittleEndian.Uint64(blob[ptr+9 : ptr+17])
		if kind == BlockKindText {
			dataCount++
			if offset < lastDataOffset {
				t.Fatalf("data offsets not monotonic")
			}
			lastDataOffset = offset
		}
		ptr += tocEntSize
	}
	if dataCount != 2 {
		t.Fatalf("expected 2 data entries, got %d", dataCount)
	}
}

func TestEncryptedSaveRequiresPasswordOnLoad(t *testing.T) {
	doc := NewDocument("author", "enc")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   1,
		Kind: BlockKindText,
		Text: &TextBlock{UTF8: []byte("secret"), Runs: []StyleRun{{Start: 0, End: 6, Attr: StyleAttr{FontSizePt: 12}}}},
	})

	path := filepath.Join(t.TempDir(), "encrypted.sqdoc")
	err := SaveWithOptions(path, doc, SaveOptions{
		Compression: true,
		Encryption:  EncryptionOptions{Enabled: true, Password: "hunter2"},
	})
	if err != nil {
		t.Fatalf("save encrypted failed: %v", err)
	}

	if _, err := LoadWithOptions(path, LoadOptions{}); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("expected ErrPasswordRequired, got %v", err)
	}
	if _, err := LoadWithOptions(path, LoadOptions{Password: "wrong"}); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
	if _, err := LoadWithOptions(path, LoadOptions{Password: "hunter2"}); err != nil {
		t.Fatalf("expected successful decrypt load, got %v", err)
	}
}

func TestInspectEnvelopeFlags(t *testing.T) {
	doc := NewDocument("author", "flags")
	doc.Blocks = append(doc.Blocks, Block{
		ID:   1,
		Kind: BlockKindText,
		Text: &TextBlock{UTF8: []byte("abc"), Runs: []StyleRun{{Start: 0, End: 3, Attr: StyleAttr{FontSizePt: 12}}}},
	})

	plain := filepath.Join(t.TempDir(), "plain.sqdoc")
	if err := SaveWithOptions(plain, doc, SaveOptions{}); err != nil {
		t.Fatalf("save plain failed: %v", err)
	}
	info, err := InspectEnvelope(plain)
	if err != nil {
		t.Fatalf("inspect plain failed: %v", err)
	}
	if info.Wrapped {
		t.Fatalf("expected plain file to be unwrapped")
	}

	secure := filepath.Join(t.TempDir(), "secure.sqdoc")
	if err := SaveWithOptions(secure, doc, SaveOptions{
		Compression: true,
		Encryption:  EncryptionOptions{Enabled: true, Password: "pw"},
	}); err != nil {
		t.Fatalf("save secure failed: %v", err)
	}
	info, err = InspectEnvelope(secure)
	if err != nil {
		t.Fatalf("inspect secure failed: %v", err)
	}
	if !info.Wrapped || !info.Compressed || !info.Encrypted {
		t.Fatalf("unexpected envelope flags: %#v", info)
	}
}

func TestRoundTripPreservesDocumentSettingsAndFontFamily(t *testing.T) {
	doc := NewDocument("author", "settings")
	doc.Metadata.PagedMode = true
	doc.Metadata.ParagraphGap = 14
	doc.Metadata.PreferredFontFamily = FontFamilyMonospace
	doc.Blocks = append(doc.Blocks, Block{
		ID:   1,
		Kind: BlockKindText,
		Text: &TextBlock{
			UTF8: []byte("hello world"),
			Runs: []StyleRun{{
				Start: 0,
				End:   11,
				Attr: StyleAttr{
					FontFamily: FontFamilySerif,
					FontSizePt: 14,
					ColorRGBA:  0x202020FF,
				},
			}},
		},
	})

	path := filepath.Join(t.TempDir(), "settings_font.sqdoc")
	if err := Save(path, doc); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !loaded.Metadata.PagedMode {
		t.Fatalf("expected paged mode true")
	}
	if loaded.Metadata.ParagraphGap != 14 {
		t.Fatalf("paragraph gap mismatch: got %d", loaded.Metadata.ParagraphGap)
	}
	if loaded.Metadata.PreferredFontFamily != FontFamilyMonospace {
		t.Fatalf("preferred font mismatch: got %d", loaded.Metadata.PreferredFontFamily)
	}
	if got := loaded.Blocks[0].Text.Runs[0].Attr.FontFamily; got != FontFamilySerif {
		t.Fatalf("run font family mismatch: got %d", got)
	}
}
