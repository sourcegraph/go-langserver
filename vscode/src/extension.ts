/* --------------------------------------------------------------------------------------------
 * Copyright (c) Microsoft Corporation. All rights reserved.
 * Licensed under the MIT License. See License.txt in the project root for license information.
 * ------------------------------------------------------------------------------------------ */
'use strict';

import { Disposable, ExtensionContext, Uri, workspace } from 'vscode';
import { GoLanguageClient } from './go-language-client';
import * as _ from 'lodash';

export function activate(context: ExtensionContext) {
	// Update GOPATH, GOROOT, etc., when config changes.
	updateEnvFromConfig();
	context.subscriptions.push(workspace.onDidChangeConfiguration(() => {
		updateEnvFromConfig();
	}));

	let languageClient = new GoLanguageClient().start();
	context.subscriptions.push(languageClient);
}

function updateEnvFromConfig() {
	const conf = workspace.getConfiguration('go');

	// // search for refreshTrace
	// const langserverConf = workspace.getConfiguration('langserver-go');
	// let keys = ['trace.server', 'transportKind.server'];
	// _.each(keys, key => {
	// 	let val = langserverConf.get(key);
	// 	console.info('langserverConf - key: %s, value: %s', key, val);
	// });

	if (conf['goroot']) {
		process.env.GOROOT = conf['goroot'];
	}
	if (conf['gopath']) {
		process.env.GOPATH = conf['gopath'];
	}
}


	// private refreshTrace(connection: IConnection, sendNotification: boolean = false): void {
	// 	let config = Workspace.getConfiguration(this._id);
	// 	let trace: Trace = Trace.Off;
	// 	if (config) {
	// 		trace = Trace.fromString(config.get('trace.server', 'off'));
	// 	}
	// 	this._trace = trace;
	// 	connection.trace(this._trace, this._tracer, sendNotification);
	// }

