import * as WebSocket from 'ws';
import { Logger } from './logger';
import * as Types from './types';
import * as Messages from './messages';
import * as Utils from './utils';

const LOGGER = Logger.create(__filename, __dirname);
const PORT_WEBSOCKET = process.env.PORT_WEBSOCKET || '4389';

LOGGER.log('PORT_WEBSOCKET', PORT_WEBSOCKET);

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

        // let uri = `ws://localhost:${PORT_WEBSOCKET}/`;
        let uri = `ws://echo.websocket.org/`;
        LOGGER.log('uri: %s', uri);

        let ws = new WebSocket(uri);

        let recvCount: number = 0;
        ws.on('message', (data: any, flags: { binary: boolean }) => {
            recvCount++;
            LOGGER.log('<--recv message - data: %j, flags: %j, receivedMsgCount: %d', data, flags, recvCount);
            if (recvCount === Messages.Test.COUNT) {
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

        Utils.Timeouts.Fail(errorHandler);
    });
};

export default run;