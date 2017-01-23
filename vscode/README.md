# Visual Studio Code extension using the Go language server [![Build Status](https://travis-ci.org/sourcegraph/go-langserver.svg)](https://travis-ci.org/sourcegraph/go-langserver)

This extension uses the Go language server in this repository to provide Go language support to Visual Studio Code.

**Status: experimental.** You should still use [Microsoft/vscode-go](https://github.com/Microsoft/vscode-go) for everyday editing.

[**Open in Sourcegraph**](https://sourcegraph.com/github.com/sourcegraph/go-langserver/-/tree/vscode)

## Using this extension

1. Run `npm install`.
1. Run `npm run isolated-vscode` to start a new isolated VSCode instance with this plugin installed. Use `npm run isolated-vscode -- /path/to/mydir/` to open the editor to a specific directory.
1. Open a `.go` file and hover over text to start using the Go language server.

To view a language server's stderr output in VSCode, select View → Output. To debug further, see the "Hacking on this extension" section below.

After updating the binary for a language server (during development or after an upgrade), just kill the process (e.g., `killall langserver-go`). VSCode will automatically restart and reconnect to the language server process.

> **Note for those who use VSCode as their primary editor:** This extension's functionality conflicts with that provided by [Microsoft/vscode-go](https://github.com/Microsoft/vscode-go). You should not use both at the same time (or else you'll see duplicate hovers, etc.) The `npm run isolated-vscode` script launches an separate instance of VSCode and stores its config in `../.vscode-dev`, so you can avoid needing to disable Microsoft/vscode-go in your main VSCode environment while hacking on this extension.

## Hacking on this extension

1. Run `npm install` in this directory.
1. Open this directory by itself in Visual Studio Code.
1. Hit F5 to open a new VSCode instance in a debugger running this extension. (This is equivalent to going to the Debug pane on the left and running the "Launch Extension" task.)

See the [Node.js example language server tutorial](https://code.visualstudio.com/docs/extensions/example-language-server) for a good introduction to building VSCode extensions.
