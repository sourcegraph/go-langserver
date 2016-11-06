import * as WebSocket from 'ws';
import * as Types from './types';

const PORT_WEBSOCKET = process.env.PORT_WEBSOCKET || '4389';
console.info('---PORT_WEBSOCKET', PORT_WEBSOCKET);

const MESSAGE_NAME: string = 'some_rpc_message';
const MESSAGE_COUNT: number = 3;

const FAILURE_DELAY = 10 * 1000;

const makeUrl = () => {
    return 'ws://localhost:' + PORT_WEBSOCKET + '/';
};
const URL = makeUrl();

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

            console.log('--message:', data, flags, messageCount);
        });

        ws.on('open', () => {
            Array.from(Array(MESSAGE_COUNT).keys()).map((messageIndex) => {
                let messageName = `MESSAGE_NAME-${messageIndex}`;
                console.log('---sending message:', messageIndex, messageName);
                ws.send(messageName);
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