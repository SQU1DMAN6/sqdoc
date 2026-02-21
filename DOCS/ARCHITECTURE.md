# Architecture

Current implementation provides:
- `pkg/sqdoc`: v1 binary format and validation.
- `internal/editor`: document editing state model.
- `internal/render`: software framebuffer painter.
- `internal/ui`: writer-like shell layout renderer and theme tokens.
- `internal/platform/*`: cross-platform backend interface and initial backend scaffolding.
- `internal/app`: event loop orchestration, keyboard commands, undo/redo snapshots, help overlay, and the toggleable block-map panel.

SQDoc v1 currently stores text payloads separately from formatting directives, with both addressed by the TOC for random access.
