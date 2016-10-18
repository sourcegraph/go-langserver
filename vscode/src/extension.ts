/* --------------------------------------------------------------------------------------------
 * Copyright (c) Microsoft Corporation. All rights reserved.
 * Licensed under the MIT License. See License.txt in the project root for license information.
 * ------------------------------------------------------------------------------------------ */
'use strict';

import * as net from 'net';
import { Disposable, ExtensionContext, Uri, workspace } from 'vscode';
import * as vscode from 'vscode';
import { LanguageClient, LanguageClientOptions, SettingMonitor, ServerOptions, ErrorAction, ErrorHandler, CloseAction, TransportKind } from 'vscode-languageclient';
import * as GitUtils from "./git-utils";
import * as SourcegraphUrls from "./sourcegraph-urls";

const child_process = require('child_process');
const path = require('path');
const fetch = require('node-fetch');
const node_url = require('url');

let langServer: LanguageClient;

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
				'-trace', '-logfile=/tmp/langserver-go.log',
			],
		},
		{
			documentSelector: ['go'],
			uriConverters: {
				// Apply file:/// scheme to all file paths.
				code2Protocol: (uri: Uri): string => (uri.scheme ? uri : uri.with({ scheme: 'file' })).toString(),
				protocol2Code: (uri: string) => Uri.parse(uri),
			},
		}
	);
	langServer = c;
	const registeredCommand = vscode.commands.registerCommand("sourcegraph.findExternalRefs", openExternalReferences, this);
	context.subscriptions.push(c.start(), registeredCommand);

	// Update GOPATH, GOROOT, etc., when config changes.
	updateEnvFromConfig();
	context.subscriptions.push(workspace.onDidChangeConfiguration(updateEnvFromConfig));
}

function openExternalReferences(args, args1) {
	let e = vscode.window.activeTextEditor;
    let d = e.document;
	let pos = e.selection.start;
	
	// Need the langserver variable c here somehow
	const thenable = langServer.sendRequest({method: "textDocument/definition"}, {
		id: 0,
		textDocument: {
			uri: args._formatted,
		},
		position: {
			line: pos.line,
			character: pos.character,
		},
	});
	thenable.then((response) => {
		const url = createExternalRefsUrl(response[0].uri, response[0].range.start.line, response[0].range.start.character);
		url.then((url) => {
			if(url) {
				child_process.exec(`open ${url}`);
			}
		}
		);
	});
}

function createExternalRefsUrl(uri: string, row: number, col:number) :Promise<String> {
	const directory = path.dirname(uri).replace("file://", "");
	const topLevelGitDirectory = GitUtils.getTopLevelGitDirectory(directory);
	let gitUrl = GitUtils.cleanGitUrl(GitUtils.getGitUrl(directory));
	let gitRev = GitUtils.getGitCommitHash(topLevelGitDirectory);
	let filePath = uri.substr(uri.indexOf(topLevelGitDirectory)+topLevelGitDirectory.length+1);

	// Special case homebrew
	if (gitUrl === "github.com/Homebrew/homebrew") {
		gitUrl = "github.com/golang/go";
		gitRev = GitUtils.getContentsOfFile(`${path.join(topLevelGitDirectory, "go")}/VERSION`);
		filePath = filePath.substr(3);
	}

	const url = `https://sourcegraph.com/.api/repos/${gitUrl}@${gitRev}/-/hover-info?file=${filePath}&line=${row}&character=${col}`;
	return fetch(url).then(resp => resp.json()).then((resp) => {
		if (resp && (resp as any).def) {
			// TODO(uforic): Remove this when we remove srclib dependency. Fix a special case for golang/go. 
			const def = resp.def;
			if (def.Repo === "github.com/golang/go" && def.Unit && def.Unit.startsWith("github.com/golang/go/src/")) {
				def.Unit = def.Unit.replace("github.com/golang/go/src/", "");
			}
			const url = node_url.resolve(`https://sourcegraph.com`,`${SourcegraphUrls.urlToDefInfo((resp as any).def)}`);
			return url;
		}
	}).catch((err) => {
		console.log(err);
	});
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
