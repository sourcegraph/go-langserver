import { Logger } from './logger';
const LOGGER = Logger.create(__filename, __dirname);

export const Delays = {
    Fail: 10 * 1000
};

let Fail = (errorHandler: () => void, delay: number = Delays.Fail) => {
    setTimeout(() => {
        let TIMEOUT_SECS = delay / 1000 + 's';
        LOGGER.log('Test timeout in: %d', TIMEOUT_SECS);

        errorHandler();
    }, delay);
}

export const Timeouts = {
    Fail
};