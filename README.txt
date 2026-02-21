Priority    Task
High        Draft binary layout of SQDoc blocks + index
High        Build HEADER + METADATA schema
High        Build SIDE SQDoc document editor with basic read, write, and text editing capability in an intuitive GUI
High        Allow for multiple text types (dynamic font sizes, text decorations)
Medium      Implement block types (text/media/style/scripts)
Medium      SIDE: render blocks from index
Low         Media embedding + compression
Low         Versioning / patch system

Think of DOCX, but not bloated.

Structure of SQDoc file:
myDocument.sqdoc
- Header + Metadata (26-byte Magic to identify document type, info like Author, date, things to remember)
- Index (shows the padding and offset of blocks to determine their location and allow for content reading and writing)
- Formatting directive block (distinct, human-unreadable, accessed by the index, treated like a data block. Similar to CSS, contains info about the style of the text in the data blocks, like colour, font size, maybe . Pointers and offsets are to be written like in the index.)
- Data blocks (where the text sits, raw format, maybe some Markdown or HTML-like modifiers for complex things like hyperlinks, tables, images, labels, and lines)

This structure allows data to be appended LITERALLY ANYWHERE and still be readable, as long as it can be seen and labelled by the index, parsed by the Formatting Directive Block, and doesn't get in the way of the index, directive blocks, or Magic.
The project aims to be made for Windows and Linux, with an intuitive, beautiful, and easy-to-build GUI. It aims to be fast, simple, lightweight, and efficient. Avoid using bulky languages like Python, and excessively complex languages like Rust.