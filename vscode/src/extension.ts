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
import { Range, SymbolInformation, Location, TextEdit, WorkspaceChange, TextEditChange, TextDocumentIdentifier } from 'vscode-languageserver-types';

const child_process = require('child_process');
const path = require('path');
const fetch = require('node-fetch');
const node_url = require('url');
const fs = require('fs');
const deep_equal = require('deep-equal');

let langServer: LanguageClient = null;

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

function findSymbol(symbols: [SymbolInformation], location: EnhancedLocation): SymbolInformation|null {
	for (let i = 0; i < symbols.length; i++) {
		console.log(symbols[i].location.uri);
	}
	for (let i = 0; i < symbols.length; i++) { 
		const symbolInformation = symbols[i];
		if (symbolInformation.location.uri !== location.uri) {
			continue;
		}
		console.log("Found uri");
		if (!deep_equal(symbolInformation.location.range.start, location.range.start)) {
			continue;
		}
		return symbolInformation;
	}
	return null;
}

export interface EnhancedLocation extends Location {
	name: string;
}

function openExternalReferences(args: any): void {
	let e = vscode.window.activeTextEditor;
	const defLocationArgs = {
		id: 0,
		textDocument: {
			uri: args._formatted,
		},
		position: {
			line: e.selection.start.line,
			character: e.selection.start.character,
		},
	};
	const defLocationRequest = langServer.sendRequest({method: "textDocument/definition"}, defLocationArgs);
	defLocationRequest.then((response: [EnhancedLocation]) => {
		const location = response[0];
		const workspaceSymbolParams = {
			query: location.name,
		};
		const workspaceSymbolRequest = langServer.sendRequest({method: "workspace/symbol"}, workspaceSymbolParams);
		workspaceSymbolRequest.then((workspaceSymbols) => {
			const foundSymbol = findSymbol((workspaceSymbols as [SymbolInformation]), location);
			if (!foundSymbol) {
				console.log(`Couldn't find symbol ${location.name}`);
				return;
			}
			const url = createExternalRefsUrl(foundSymbol);
			if (url) {
				console.log(`Def landing URL: ${url}`);
				child_process.exec(`open ${url}`);
			}
		});
	});
}

function getGitUriFromFileUri(uri: string) {
	if (uri.indexOf("/vendor/") >= 0) {
		const gitUriAndPath = uri.split("/vendor/")[1];
		if (gitUriAndPath.startsWith("github.com/")) {
			return gitUriAndPath.split("/").slice(0, 3).join("/");
		}
		return gitUriAndPath.split("/").slice(0, 2).join("/");
	}
	const directory = path.dirname(uri).replace("file://", "");
	return GitUtils.cleanGitUrl(GitUtils.getGitUrl(directory));
}

function getPathFromFileUri(uri: string, gitUri: string) {
	if (uri.indexOf("/vendor/") >= 0) {
		const gitUriAndPath = uri.split("/vendor/")[1];
		if (gitUriAndPath.startsWith("github.com/")) {
			return gitUriAndPath.split("/").slice(0, -1).join("/");
		}
		return gitUriAndPath.split("/").slice(0, -1).join("/");
	}
	const directory = path.dirname(uri).replace("file://", "");
	const topLevelDirectory = `${GitUtils.getTopLevelGitDirectory(directory)}/`;
	return `${gitUri}/${directory.split(topLevelDirectory)[1]}`;
}

function createExternalRefsUrl(symbolInformation: SymbolInformation): string {
	let gitUri = getGitUriFromFileUri(symbolInformation.location.uri);
	let repoPkg = getPathFromFileUri(symbolInformation.location.uri, gitUri);

	let containerSymbol = symbolInformation.name;
	if (symbolInformation.containerName && symbolInformation.containerName[0].toUpperCase() === symbolInformation.containerName[0]) {
		containerSymbol = `${symbolInformation.containerName}/${containerSymbol}`;
	}

	const UNIX_GO_INSTALL_LOC = "file:///usr/local/go";
	if (symbolInformation.location.uri.startsWith(UNIX_GO_INSTALL_LOC)) {
		repoPkg = repoPkg.substr(`${gitUri}/go/src/`.length);
		gitUri = "github.com/golang/go";
	}

	const url = constructUrl(gitUri, repoPkg, containerSymbol);
	return url;
}

function constructUrl(repoUrl: string, repoPkg: string, containerSymbol: string) {
	return `https://sourcegraph.com/${repoUrl}/-/info/GoPackage/${repoPkg}/-/${containerSymbol}`;
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
