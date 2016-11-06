import Connect from './connect';
import * as Types from './types';

console.log('Running tests');

// execute all tests - they are promise-based
let testResults = ([
    Connect
]).map(test => test());

// wait for all promises to complete or an error occurs
Promise.all(testResults).then((results: Types.ResultType[]) => {
    results.map((result: Types.ResultType) => {
        console.log('Test - result: ', result);
    });

    process.exit();
}).catch((errorTest: Types.ResultType) => {
    console.error('Some error: ', errorTest);

    process.exit(1);
});
