import * as vscode from 'vscode';
import * as path from 'path';
import * as cp from 'child_process';
import * as fs from 'fs';
import { LanguageClient, LanguageClientOptions, ServerOptions } from 'vscode-languageclient/node';

let client: LanguageClient;

export function activate(context: vscode.ExtensionContext) {
    console.log('AGL extension is now active');

    const config = vscode.workspace.getConfiguration('agl');
    // Use absolute path to the language server
    const serverPath = 'agl-lsp';
    
    // Debug logging
    console.log('Server path:', serverPath);
    console.log('Server exists:', fs.existsSync(serverPath));

    const serverOptions: ServerOptions = {
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

    const clientOptions: LanguageClientOptions = {
        documentSelector: [{ scheme: 'file', language: 'agl' }],
        synchronize: {
            fileEvents: vscode.workspace.createFileSystemWatcher('**/*.agl')
        },
        outputChannel: vscode.window.createOutputChannel('AGL Language Server')
    };

    client = new LanguageClient(
        'agl',
        'AGL Language Server',
        serverOptions,
        clientOptions
    );

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

export function deactivate() {
    if (client) {
        return client.stop();
    }
} 