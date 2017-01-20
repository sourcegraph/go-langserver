import * as WebSocket from 'ws';
import * as Types from './types';

let PORT_WEBSOCKET = process.env.PORT_WEBSOCKET;
console.log('PORT_WEBSOCKET:', PORT_WEBSOCKET);

let makeUrl = () => {
	let port = PORT_WEBSOCKET || '9999';
	return 'ws://localhost:' + port + '/ws';
};

let url = makeUrl();

let langserver = new WebSocket(url);
langserver.on('open', () => {
	let messages_test = [
		'1some_rpc_message',
		'2some_rpc_message',
		'3some_rpc_message'
	];
	messages_test.map((message_test) => {
		console.log('sending message: %O', message_test);
		langserver.send(message_test);
	});
});
langserver.on('message', (data, flags) => {
	// flags.binary will be set if a binary data is received.
	// flags.masked will be set if the data was masked.
});

let run: Types.TestRun = () => {
    return Promise.resolve(true);
};

export default run;

