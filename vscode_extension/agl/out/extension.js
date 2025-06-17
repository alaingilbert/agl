"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = require("vscode");
const node_1 = require("vscode-languageclient/node");
let client;
function activate(context) {
    console.log('AGL extension is now active');
    const config = vscode.workspace.getConfiguration('agl');
    const serverPath = config.get('languageServerPath') || 'agl-lsp';
    const serverOptions = {
        command: serverPath,
        args: [],
        options: {
            shell: true
        }
    };
    const clientOptions = {
        documentSelector: [{ scheme: 'file', language: 'agl' }],
        synchronize: {
            fileEvents: vscode.workspace.createFileSystemWatcher('**/*.agl')
        }
    };
    client = new node_1.LanguageClient('agl', 'AGL Language Server', serverOptions, clientOptions);
    // Start the client and add it to the subscriptions
    client.start().then(() => {
        context.subscriptions.push({
            dispose: () => client.stop()
        });
    });
    // Register commands
    let disposable = vscode.commands.registerCommand('agl.restartLanguageServer', () => {
        client.restart();
    });
    context.subscriptions.push(disposable);
}
function deactivate() {
    if (client) {
        return client.stop();
    }
}
//# sourceMappingURL=extension.js.map