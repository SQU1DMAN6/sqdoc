package editor

import (
	"testing"

	"sqdoc/pkg/sqdoc"
)

func TestSplitBlockAtCaret(t *testing.T) {
	s := NewState(sqdoc.NewDocument("", ""))
	if err := s.UpdateCurrentText("hello world"); err != nil {
		t.Fatal(err)
	}
	s.CaretByte = 5
	s.SplitBlockAtCaret()

	if s.BlockCount() != 2 {
		t.Fatalf("expected 2 blocks, got %d", s.BlockCount())
	}
	if got := s.AllBlockTexts()[0]; got != "hello" {
		t.Fatalf("unexpected first block: %q", got)
	}
	if got := s.AllBlockTexts()[1]; got != " world" {
		t.Fatalf("unexpected second block: %q", got)
	}
}

func TestBackspaceMergesWithPreviousBlock(t *testing.T) {
	s := NewState(sqdoc.NewDocument("", ""))
	if err := s.UpdateCurrentText("a"); err != nil {
		t.Fatal(err)
	}
	s.SplitBlockAtCaret()
	if err := s.UpdateCurrentText("b"); err != nil {
		t.Fatal(err)
	}
	s.CaretByte = 0
	s.Backspace()

	if s.BlockCount() != 1 {
		t.Fatalf("expected 1 block, got %d", s.BlockCount())
	}
	if got := s.AllBlockTexts()[0]; got != "ab" {
		t.Fatalf("unexpected merged text: %q", got)
	}
}

func TestInsertAndDelete(t *testing.T) {
	s := NewState(sqdoc.NewDocument("", ""))
	if err := s.UpdateCurrentText("abcd"); err != nil {
		t.Fatal(err)
	}
	s.CaretByte = 2
	if err := s.InsertTextAtCaret("X"); err != nil {
		t.Fatal(err)
	}
	if got := s.CurrentText(); got != "abXcd" {
		t.Fatalf("unexpected insert result: %q", got)
	}
	s.DeleteForward()
	if got := s.CurrentText(); got != "abXd" {
		t.Fatalf("unexpected delete result: %q", got)
	}
}

func TestSelectionDeleteAcrossBlocks(t *testing.T) {
	s := NewState(sqdoc.NewDocument("", ""))
	if err := s.UpdateCurrentText("alpha"); err != nil {
		t.Fatal(err)
	}
	s.SplitBlockAtCaret()
	if err := s.UpdateCurrentText("beta"); err != nil {
		t.Fatal(err)
	}

	s.SetCaret(0, 2)
	s.EnsureSelectionAnchor()
	s.SetCaret(1, 2)
	s.UpdateSelectionFromCaret()

	if !s.DeleteSelection() {
		t.Fatalf("expected selection delete")
	}
	if s.BlockCount() != 1 {
		t.Fatalf("expected 1 block, got %d", s.BlockCount())
	}
	if got := s.CurrentText(); got != "alta" {
		t.Fatalf("unexpected merged text: %q", got)
	}
}

func TestInsertMultilineCreatesBlocks(t *testing.T) {
	s := NewState(sqdoc.NewDocument("", ""))
	if err := s.UpdateCurrentText("abc"); err != nil {
		t.Fatal(err)
	}
	s.CaretByte = 1
	if err := s.InsertTextAtCaret("X\nY"); err != nil {
		t.Fatal(err)
	}
	if s.BlockCount() != 2 {
		t.Fatalf("expected 2 blocks, got %d", s.BlockCount())
	}
	all := s.AllBlockTexts()
	if all[0] != "aX" {
		t.Fatalf("unexpected first block: %q", all[0])
	}
	if all[1] != "Ybc" {
		t.Fatalf("unexpected second block: %q", all[1])
	}
}

func TestInlineStyleAppliedToSelection(t *testing.T) {
	s := NewState(sqdoc.NewDocument("", ""))
	if err := s.UpdateCurrentText("hello world"); err != nil {
		t.Fatal(err)
	}
	s.SetCaret(0, 6)
	s.EnsureSelectionAnchor()
	s.SetCaret(0, 11)
	s.UpdateSelectionFromCaret()
	s.ToggleBold()
	s.ClearSelection()

	s.SetCaret(0, 7)
	if !s.CurrentStyleAttr().Bold {
		t.Fatalf("expected selected segment to be bold")
	}
	s.SetCaret(0, 1)
	if s.CurrentStyleAttr().Bold {
		t.Fatalf("expected non-selected segment to remain non-bold")
	}
}

func TestWordMovement(t *testing.T) {
	s := NewState(sqdoc.NewDocument("", ""))
	if err := s.UpdateCurrentText("hello brave world"); err != nil {
		t.Fatal(err)
	}
	s.CaretByte = len("hello brave world")
	s.MoveCaretWordLeft()
	if s.CaretByte != len("hello brave ") {
		t.Fatalf("unexpected first word-left caret: %d", s.CaretByte)
	}
	s.MoveCaretWordLeft()
	if s.CaretByte != len("hello ") {
		t.Fatalf("unexpected second word-left caret: %d", s.CaretByte)
	}
	s.MoveCaretWordRight()
	if s.CaretByte != len("hello brave") {
		t.Fatalf("unexpected word-right caret: %d", s.CaretByte)
	}
}
