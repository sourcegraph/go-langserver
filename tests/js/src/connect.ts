import * as WebSocket from 'ws';
import { Logger } from './logger';
import * as Types from './types';
import * as Messages from './messages';

let filename: string = __filename.slice(__dirname.length + 1);
const LOGGER = new Logger(filename);

const PORT_WEBSOCKET = process.env.PORT_WEBSOCKET || '4389';
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

        let receivedMsgCount: number = 0;
        ws.on('message', (data: any, flags: { binary: boolean }) => {
            // flags.binary will be set if a binary data is received.
            // flags.masked will be set if the data was masked.

            receivedMsgCount++;
            LOGGER.log('<--recv message - data: %j, flags: %j, receivedMsgCount: %d', data, flags, receivedMsgCount);
            if (receivedMsgCount === Messages.Test.COUNT) {
                succesHandler();
            }
        });

        ws.on('open', () => {
            // send all the msgs: init and the rest...
            let msgs = [Messages.Test.INIT, ...Messages.Test.REST];
            msgs.map(msg => {
                let asRequest = msg.toRequest();
                LOGGER.log('-->sending message: ', asRequest);
                ws.send(asRequest);
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