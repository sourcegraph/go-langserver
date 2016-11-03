let WebSocket = require('ws');

let makeUrl = function () {
	let port = '9999';
	return 'ws://localhost:' + port + '/ws';
}

let url = makeUrl();

let langserver = new WebSocket(url);
langserver.on('open', function open() {
	let messages_test = [
		'1some_rpc_message',
		'2some_rpc_message',
		'3some_rpc_message'
	];
	messages_test.map(function (message_test) {
		console.log('sending message: %O', message_test);
		langserver.send(message_test);
	});
});

langserver.on('message', function (data, flags) {
	// flags.binary will be set if a binary data is received.
	// flags.masked will be set if the data was masked.
});

