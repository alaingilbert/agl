Syntax highlight guide for vscode
https://code.visualstudio.com/api/language-extensions/overview

Scanner/Token based on commit d166a0b03e88e3ffe17a5bee4e5405b5091573c6

To update agl-lsp, cd in the language_server folder, then
```sh
go mod vendor && go build -o agl-lsp main.go
```

To update the wasm build (playground) 
```sh
GOOS=js GOARCH=wasm go build -o docs/main.wasm main.go
```

LSP spec https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/