/*---------------------------------------------------------
 * Copyright (C) Microsoft Corporation. All rights reserved.
 * Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------*/

import * as assert from 'assert';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import * as vscode from 'vscode';

suite('Go extension tests', () => {
	const gopath = fs.mkdtempSync(path.join(os.tmpdir(), 'vscode-go-langserver-'));
	process.env['GOPATH'] = gopath; // TODO(sqs): set using config, not env vars directly
	const pkgDir = path.join(gopath, 'src', 'test', 'p');
	const filePath = path.join(pkgDir, 'a.go');

	suiteSetup(() => {
		fs.mkdirSync(path.join(pkgDir, '../..'));
		fs.mkdirSync(path.join(pkgDir, '..'));
		fs.mkdirSync(pkgDir);
		fs.writeFileSync(filePath, "package p; func A() {}; var _ = A");
	});

	suiteTeardown(() => {
		fs.unlinkSync(filePath);
		fs.rmdirSync(pkgDir);
		fs.rmdirSync(path.join(pkgDir, '..'));
		fs.rmdirSync(path.join(pkgDir, '../..'));
		fs.rmdirSync(gopath);
	});

	test('hover', (done) => {
		let testCases: [vscode.Position, string][] = [
			[new vscode.Position(0, 8), 'package p'],
			[new vscode.Position(0, 16), 'func A()'],
			[new vscode.Position(0, 32), 'func A()'],
		];
		let uri = vscode.Uri.file(filePath);
		vscode.extensions.getExtension("sourcegraph.Go").activate().then(() => {
			vscode.workspace.openTextDocument(uri).then((textDocument) => {
				let promises = testCases.map(([position, wantHover]) => {
					return vscode.commands.executeCommand('vscode.executeHoverProvider', textDocument.uri, position).then((res: vscode.Hover[]) => {
						assert.equal(wantHover, (res[0].contents[res[0].contents.length - 1] as {language: string; value: string}).value);
					});
				});
				return Promise.all(promises);
			}, (err) => {
				assert.ok(false, `error in vscode.workspace.openTextDocument: ${err}`);
			}).then(() => done(), done);
		});
	});
});
