```sh
cd language_server
go mod vendor && go build -o agl-lsp main.go
mv agl-lsp /usr/local/bin/agl-lsp
```

Then you can install this extension in vscode
https://github.com/alaingilbert/agl/tree/master/vscode_extension/agl
(cmd+shift+p) `install extension from location`