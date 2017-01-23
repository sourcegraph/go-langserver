/* --------------------------------------------------------------------------------------------
 * Copyright (c) Microsoft Corporation. All rights reserved.
 * Licensed under the MIT License. See License.txt in the project root for license information.
 * ------------------------------------------------------------------------------------------ */
'use strict';

import * as net from 'net';

import { Disposable, ExtensionContext, Uri, workspace } from 'vscode';
import { LanguageClient, LanguageClientOptions, SettingMonitor, ServerOptions, ErrorAction, ErrorHandler, CloseAction, TransportKind } from 'vscode-languageclient';

export function activate(context: ExtensionContext) {
	const c = new LanguageClient(
		'langserver-go',
		{
			command: 'langserver-go',
			args: [
				'-mode=stdio',

				// Uncomment for verbose logging to the vscode
				// "Output" pane and to a temporary file:
				//
				// '-trace', '-logfile=/tmp/langserver-go.log',
			],
		},
		{
			documentSelector: ['go'],
			uriConverters: {
				// Apply file:/// scheme to all file paths.
				code2Protocol: (uri: Uri): string => (uri.scheme ? uri : uri.with({scheme: 'file'})).toString(),
				protocol2Code: (uri: string) => Uri.parse(uri),
			},
		}
	);
	context.subscriptions.push(c.start());

	// Update GOPATH, GOROOT, etc., when config changes.
	updateEnvFromConfig();
	context.subscriptions.push(workspace.onDidChangeConfiguration(updateEnvFromConfig));
}

function updateEnvFromConfig() {
	const conf = workspace.getConfiguration('go');
	if (conf['goroot']) {
		process.env.GOROOT = conf['goroot'];
	}
	if (conf['gopath']) {
		process.env.GOPATH = conf['gopath'];
	}
}
