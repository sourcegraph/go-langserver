import * as WebSocket from 'ws';
import { Logger } from './logger';
import * as Types from './types';
import * as Messages from './messages';

let filename: string = __filename.slice(__dirname.length + 1);
const LOGGER = new Logger(filename);

const PORT_WEBSOCKET = process.env.PORT_WEBSOCKET || '4389';
const MESSAGE_NAME: string = 'some_rpc_message';
const MESSAGE_COUNT: number = 2; // include the init message
const FAILURE_DELAY = 10 * 1000;

LOGGER.log('PORT_WEBSOCKET', PORT_WEBSOCKET);

const makeUrl = () => {
    return 'ws://localhost:' + PORT_WEBSOCKET + '/';
};
const WS_URL = makeUrl();

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

        let ws = new WebSocket(WS_URL);

        let messageCount: number = 0;
        ws.on('message', (data: any, flags: { binary: boolean }) => {
            // flags.binary will be set if a binary data is received.
            // flags.masked will be set if the data was masked.

            messageCount++;
            if (messageCount === MESSAGE_COUNT) {
                succesHandler();
            }

            LOGGER.log('<--recv message - data: %j, flags: %j, messageCount: %d', data, flags, messageCount);
        });

        ws.on('open', () => {
            let msgJsonInit = {
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
            };

            let rpcMessageInit = Messages.toRequest(msgJsonInit);
            LOGGER.log('-->sending message: ', rpcMessageInit);

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

                let rpcMessageTextOpen = Messages.toRequest(msgJsonTextOpen);
                LOGGER.log('-->sending message: ', rpcMessageTextOpen);

                ws.send(rpcMessageTextOpen);
            });
        });

        ws.on('error', errorHandler);
        ws.on('close', errorHandler);

        // fail otherwise
        setTimeout(() => {
            let TIMEOUT_SECS = FAILURE_DELAY / 1000 + 's';
            LOGGER.log('Test timeout in: %d', TIMEOUT_SECS);

            errorHandler();
        }, FAILURE_DELAY);
    });
};

export default run;