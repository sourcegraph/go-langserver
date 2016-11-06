import * as WebSocket from 'ws';
import * as Types from './types';

let PORT_WEBSOCKET = process.env.PORT_WEBSOCKET;
console.info('PORT_WEBSOCKET', PORT_WEBSOCKET);

let makeUrl = () => {
    let port = PORT_WEBSOCKET || '9999';
    return 'ws://localhost:' + port + '/';
};
let URL = makeUrl();

let run: Types.TestRun = () => {
    return new Promise((resolve, reject) => {
        let errorHandler = (...result: any[]) => {
            let ERROR: Types.ResultType = {
                name: 'Connect',
                pass: false,
                result
            };
            resolve(ERROR);
        };

        let succesHandler = (...result: any[]) => {
            let SUCCESS: Types.ResultType = {
                name: 'Connect',
                pass: true,
                result,
            };
            resolve(SUCCESS);
        };

        let langserver = new WebSocket(URL);

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
            succesHandler(data, flags);

            // flags.binary will be set if a binary data is received.
            // flags.masked will be set if the data was masked.
        });

        langserver.on('error', errorHandler);
        langserver.on('close', errorHandler);

        // fail in 5 seconds
        setTimeout(() => {
            errorHandler();
        }, 5000);
    });
};

export default run;