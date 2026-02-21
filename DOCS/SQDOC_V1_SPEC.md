# SQDoc v1 Binary Format

## Header (42 bytes)
- Magic: 26 bytes (`KeepCalmAndFuckTheRussians`)
- Version: `uint16` (`1`)
- Flags: `uint16`
  - bit0 (`0x0001`): random-access mode required
- TOC offset: `uint64`
- TOC count: `uint32`

No reserved padding bytes are present in the header.

## TOC Entry (25 bytes)
- Block ID: `uint64`
- Block kind: `uint8`
- Payload offset: `uint64`
- Payload length: `uint32`
- CRC32: `uint32`

## Block Kinds
- `0`: metadata
- `1`: text data block
- `2`: media (reserved)
- `3`: formatting directive block
- `4`: script (reserved)

## File Layout
The encoder writes:
1. Header
2. TOC/index payload
3. Metadata block payload
4. Formatting directive block payload
5. One or more text data block payloads

The TOC is near the start for direct random access. Data blocks are written last.

## Metadata Payload
- Author: `u32` byte length + UTF-8 bytes
- Title: `u32` byte length + UTF-8 bytes
- CreatedUnix: `int64`
- ModifiedUnix: `int64`

## Formatting Directive Payload
- Entry count: `u32`
- Repeated entries:
  - Block ID: `u64`
  - Start byte offset: `u32`
  - End byte offset: `u32`
  - Flags: `u8` (`bit0=bold`, `bit1=italic`, `bit2=underline`)
  - Font size (pt): `u16`
  - RGBA color: `u32`

The directive block is index-addressable like other payloads.

## Text Block Payload
- Text bytes: `u32` length + UTF-8 bytes

For backward compatibility with older experimental files, loaders may parse optional inline style runs if extra bytes remain, but writers store style runs in the formatting directive block only.

## Validation Rules
- Header magic and version must match.
- Random-access flag (`0x0001`) must be set.
- TOC entries must fit within file and not overlap.
- CRC32 must match each payload.
- Style runs must be non-overlapping and within text byte length.
