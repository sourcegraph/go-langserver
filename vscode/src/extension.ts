/* --------------------------------------------------------------------------------------------
 * Copyright (c) Microsoft Corporation. All rights reserved.
 * Licensed under the MIT License. See License.txt in the project root for license information.
 * ------------------------------------------------------------------------------------------ */
'use strict';

import * as net from 'net';
import {ExtensionContext, Uri, workspace} from "vscode";
import * as vscode from 'vscode';
import { LanguageClient} from "vscode-languageclient";
import * as GitUtils from "./git-utils";
import {Location, SymbolInformation} from 'vscode-languageserver-types';

const childProcess = require("child_process");
const path = require("path");
const deepEqual = require("deep-equal");
const fs = require("fs");

let langServer: LanguageClient = null;

export function activate(context: ExtensionContext): void {
	const c = new LanguageClient(
		"langserver-go",
		{
			command: "langserver-go",
			args: [
				"-mode=stdio",

				// Uncomment for verbose logging to the vscode
				// "Output" pane and to a temporary file:
				//
				"-trace", "-logfile=/tmp/langserver-go.log",
			],
		},
		{
			documentSelector: ["go"],
			uriConverters: {
				// Apply file:/// scheme to all file paths.
				code2Protocol: (uri: Uri): string => (uri.scheme ? uri : uri.with({ scheme: "file" })).toString(),
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
		const symbolInformation = symbols[i];
		if (symbolInformation.location.uri !== location.uri) {
			continue;
		}
		if (!deepEqual(symbolInformation.location.range.start, location.range.start)) {
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
			const urlPromise = createExternalRefsUrl(foundSymbol);
			urlPromise.then((url: string) => {
				if (url) {
					console.log(`Def landing URL: ${url}`);
					childProcess.exec(`open ${url}`);
				}
			});
		});
	});
}

function getGitUriFromFileUri(uri: string): Promise<string> {
	return new Promise<String>((resolve, reject) => {
		if (uri.indexOf("/vendor/") >= 0) {
			// If it is a vendor package, we need to determine the git URI
			// For GitHub packages, this is easy: 3 parts (github.com/x/y)
			// For gopkg.in, it is actually variable and dependent (gopkg.in/a[/b])
			const pathParts = uri.split("/vendor/");
			const vendorRoot = path.join(pathParts[0].replace("file://", ""), "vendor");
			const gitUriAndPath = pathParts[1];

			// Shortcut if package is on GitHub.com
			if (gitUriAndPath.startsWith("github.com/")) {
				resolve(gitUriAndPath.split("/").slice(0, 3).join("/"));
			}
			fs.readFile(path.join(vendorRoot, "vendor.json"), "utf8", function (err: Error, data: any) {
				if (err) {
					reject(err);
				}
				const vendorJson = JSON.parse(data);
				for (let i = 0; i < vendorJson.package.length; i++) {
					if (gitUriAndPath.startsWith(vendorJson.package[i].path)) {
						resolve(vendorJson.package[i].path);
					}
				}
				// default behavior, just assume URI has 3 parts
				resolve(gitUriAndPath.split("/").slice(0, 3).join("/"));
			});
			return;
		}
		const directory = path.dirname(uri).replace("file://", "");
		resolve(GitUtils.cleanGitUrl(GitUtils.getGitUrl(directory)));
	});
}

function getPathFromFileUri(uri: string, gitUri: string): string {
	if (uri.indexOf("/vendor/") >= 0) {
		const gitUriAndPath = uri.split("/vendor/")[1];
		if (gitUriAndPath.startsWith("github.com/")) {
			return gitUriAndPath.split("/").slice(0, -1).join("/");
		}
		return gitUriAndPath.split("/").slice(0, -1).join("/");
	}
	const directory = path.dirname(uri).replace("file://", "");
	const topLevelDirectory = `${GitUtils.getTopLevelGitDirectory(directory)}/`;
	const pkg = directory.split(topLevelDirectory)[1];
	return `${gitUri}${pkg ? `/${pkg}` : ""}`;
}

function createExternalRefsUrl(symbolInformation: SymbolInformation): Promise<string> {
	let gitUriPromise = getGitUriFromFileUri(symbolInformation.location.uri);
	return gitUriPromise.then((gitUri: string) => {
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
	});
}

function constructUrl(repoUri: string, repoPkg: string, containerSymbol: string): string {
	return `https://sourcegraph.com/${repoUri}/-/info/GoPackage/${repoPkg}/-/${containerSymbol}`;
}

function updateEnvFromConfig(): void {
	const conf = workspace.getConfiguration("go");
	if (conf["goroot"]) {
		process.env.GOROOT = conf["goroot"];
	}
	if (conf["gopath"]) {
		process.env.GOPATH = conf["gopath"];
	}
}
