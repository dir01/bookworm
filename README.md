# bookworm

![GitHub Workflow Status](https://github.com/dir01/bookworm/actions/workflows/on_master.yml/badge.svg)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/dir01/bookworm)
![GitHub](https://img.shields.io/github/license/dir01/bookworm)

`bookworm` allows you to :
- index your books archive, a directory containing any of:
  - `.fb2` files
  - `.zip` archives with `.fb2` files inside
  - `.epub` files
  - `.zim` archives with `.epub` files inside (e.g. a [Project Gutenberg](https://download.kiwix.org/zim/gutenberg/) library from Kiwix)
- search for books (by title or author)
- get the books (as `.fb2` or `.epub`)

Books are served in their original format when possible (an `.epub` source is
streamed as-is); conversion between `.fb2` and `.epub` uses Calibre's
`ebook-convert`.

