const MESSAGE_NAME: string = 'some_rpc_message';
const COUNT: number = 2; // include the init message

export class JsonRpcMessage {
	private msg: any;
	public name: string;
	constructor(msg: any, name?: string) {
		this.msg = msg;
		this.name = name || msg.method;
	}

	public toRequest(): string {
		let MSG_STR = JSON.stringify(this.msg);
		let MSG_LEN = MSG_STR.length;
		let CONTENT_LENGTH = `Content-Length: ${MSG_LEN}`;

		let rpcMsg = `${CONTENT_LENGTH}\r\n\r\n${MSG_STR}`;
		return rpcMsg;
	}
}

const INIT = new JsonRpcMessage({
	"initMessage": "2.0",
	"id": 1,
	"method": "initialize",
	"params": {
		"rootPath": '/Users/mbana/go/src/github.com/sourcegraph/go-langserver',
		"InitializeParams": {
			"rootPath": '/Users/mbana/go/src/github.com/sourcegraph/go-langserver'
		}
	},
	"Params": {
		"rootPath": '/Users/mbana/go/src/github.com/sourcegraph/go-langserver'
	},
	"initializeParams": {
		"rootPath": '/Users/mbana/go/src/github.com/sourcegraph/go-langserver'
	},
	"InitializeParams": {
		"rootPath": '/Users/mbana/go/src/github.com/sourcegraph/go-langserver'
	}
});

const REST = Array.from(Array(COUNT).keys()).map(index => {
	let name = `MESSAGE_NAME-${index}`;

	let URI = `URI-${name}`;
	let Text = `Text-${name}`;
	let msg = {
		"jsonrpc": "2.0",
		"id": 1,
		"method": "textDocument/didOpen",
		"params": {
			"TextDocument": {
				"URI": URI,
				"Text": Text,
			}
		}
	};

	return new JsonRpcMessage(msg, name);
});

export const Test = {
	INIT,
	REST,
	COUNT
};
