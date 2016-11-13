export class JsonRpcMessage {
	private msg: Object;
	constructor(msg: Object) {
		this.msg = msg;
	}

	public toRequest(): string {
		let MSG_STR = JSON.stringify(this.msg);
		let MSG_LEN = MSG_STR.length;
		let CONTENT_LENGTH = `Content-Length: ${MSG_LEN}`;

		let rpcMsg = `${CONTENT_LENGTH}\r\n\r\n${MSG_STR}`;
		return rpcMsg;
	}
}

// export all the test messages
// export const TEST_MESSAGES = [

// ];

export const toRequest = (msg: Object) => {
	let MSG_STR = JSON.stringify(msg);
	let MSG_LEN = MSG_STR.length;
	let CONTENT_LENGTH = `Content-Length: ${MSG_LEN}`;

	let rpcMsg = `${CONTENT_LENGTH}\r\n\r\n${MSG_STR}`;
	return rpcMsg;
};