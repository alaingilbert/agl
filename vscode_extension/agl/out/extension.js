"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = require("vscode");
const fs = require("fs");
const node_1 = require("vscode-languageclient/node");
let client;
function activate(context) {
    console.log('AGL extension is now active');
    const config = vscode.workspace.getConfiguration('agl');
    // Use absolute path to the language server
    const serverPath = 'agl-lsp';
    // Debug logging
    console.log('Server path:', serverPath);
    console.log('Server exists:', fs.existsSync(serverPath));
    const serverOptions = {
        command: serverPath,
        args: [],
        options: {
            shell: false,
            env: {
                ...process.env,
                GOTRACEBACK: 'all'
            }
        }
    };
    const clientOptions = {
        documentSelector: [{ scheme: 'file', language: 'agl' }],
        synchronize: {
            fileEvents: vscode.workspace.createFileSystemWatcher('**/*.agl')
        },
        outputChannel: vscode.window.createOutputChannel('AGL Language Server')
    };
    client = new node_1.LanguageClient('agl', 'AGL Language Server', serverOptions, clientOptions);
    // Start the client and add it to the subscriptions
    client.start().then(() => {
        console.log('Language server started successfully');
        context.subscriptions.push({
            dispose: () => client.stop()
        });
    }).catch(err => {
        console.error('Failed to start language server:', err);
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