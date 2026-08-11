# go-zim

Package `zim` implements reading support for the ZIM File Format.

Documentation at <https://pkg.go.dev/github.com/tim-st/go-zim>.

Download and install package `zim` and its tools with

>`go get -u github.com/tim-st/go-zim/...`

or download a prebuilt binary from the [release](https://github.com/tim-st/go-zim/releases)-section.

You can download a ZIM file for testing [here](https://download.kiwix.org/zim/).

## Commands

The command above installs the tools of this package to `$GOPATH/bin/`.

### zimserver

Tool for browsing a ZIM file in your webbrowser via an HTTP interface.

* Starting a ZIM server at TCP port 8080: `zimserver -filename="filename.zim" -port=8080`
* Browsing the ZIM file via Web Browser is now possible at `http://localhost:8080/`
* The last part of the URL can be used as a basic prefix search by passing the search term after the last `/` in the URL

### zimindex

Tool for creating a full text index of a given ZIM file.

### zimsearch

Tool that lists search results for a given ZIM file and text query.
If no index file created by `zimindex` is found, a builtin prefix search is used. Otherwise the index file is used to retrieve search results sorted by score, where the search result can be calculated by union or intersection operation.

### zimtext

Tool to extract clean texts from a Wikipedia ZIM file.
Each clean HTML paragraph is written on a single line in a text file.

* Extracting first 1000 clean texts from a ZIM file: `zimtext -zim="filename.zim" -txt="lines.txt" -limit=1000`
* Extracting all clean texts from a ZIM file: `zimtext -zim="filename.zim" -txt="lines.txt"`
* Extracting first 1000 clean sentences (likely a sentence) from a ZIM file: `zimtext -zim="filename.zim" -txt="lines.txt" -limit=1000 -sentences`
* Extracting all clean sentences (likely a sentence) from a ZIM file: `zimtext -zim="filename.zim" -txt="lines.txt" -sentences`
* If you want to support your language or use-case better, it's recommended to define your own Regular Expression to extract only texts you accept. The RE-Syntax is defined [here](https://github.com/google/re2/wiki/Syntax) and can be tested [here](https://regex101.com/) (select Flavor=_Golang_).

Example:
> `zimtext -zim="wikipedia_de_top_nopic_2019-08.zim" -txt="de.txt" -limit=10000 -regexFilter="^(?:\p{Lu}|\p{N})[ \pL\pN\,\;\:\-]{10,}[\.\)\]\?\"…«»›‹‘“’”]{1}$"`
