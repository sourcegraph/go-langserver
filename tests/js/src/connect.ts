import * as WebSocket from 'ws';
import * as Types from './types';

const PORT_WEBSOCKET = process.env.PORT_WEBSOCKET || '4389';
console.info('---PORT_WEBSOCKET', PORT_WEBSOCKET);

const MESSAGE_NAME: string = 'some_rpc_message';
const MESSAGE_COUNT: number = 1;

const FAILURE_DELAY = 10 * 1000;

const makeUrl = () => {
    return 'ws://localhost:' + PORT_WEBSOCKET + '/';
};
const URL = makeUrl();

const makeRpcMsg = (msgJson: Object) => {
    let MSG_STR = JSON.stringify(msgJson);
    let MSG_LEN = MSG_STR.length;
    let CONTENT_LENGTH = `Content-Length: ${MSG_LEN}`;

    let rpcMsg = `${CONTENT_LENGTH}\r\n\r\n${MSG_STR}`;
    return rpcMsg;
};

const run: Types.TestRun = () => {
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

        let ws = new WebSocket(URL);

        let messageCount: number = 0;
        ws.on('message', (data: any, flags: { binary: boolean }) => {
            // flags.binary will be set if a binary data is received.
            // flags.masked will be set if the data was masked.

            // console.log('message:', data, flags);
            messageCount++;
            if (messageCount === MESSAGE_COUNT) {
                succesHandler();
            }

            console.log('<--recv message - data: %j, flags: %j, messageCount: %d', data, flags, messageCount);
        });

        ws.on('open', () => {
            let msgJsonInit = {
                "initMessage": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "processId": '',
                    "rootPath": '',
                    "capabilities": {

                    }
                }
            };
            let rpcMessageInit = makeRpcMsg(msgJsonInit);
            console.log(`-->sending message: '${rpcMessageInit}'`);
            ws.send(rpcMessageInit);

            Array.from(Array(MESSAGE_COUNT).keys()).map((messageIndex) => {
                let messageName = `MESSAGE_NAME-${messageIndex}`;

                let URI = `URI-${messageName}`;
                let Text = `Text-${messageName}`;
                let msgJsonTextOpen = {
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

                let rpcMessageTextOpen = makeRpcMsg(msgJsonTextOpen);
                console.log(`-->sending message: '${rpcMessageTextOpen}'`);
                ws.send(rpcMessageTextOpen);
            });
        });

        ws.on('error', errorHandler);
        ws.on('close', errorHandler);

        // fail otherwise
        setTimeout(() => {
            console.log('---Test timeout in ' + FAILURE_DELAY / 1000 + 's');

            errorHandler();
        }, FAILURE_DELAY);
    });
};

export default run;