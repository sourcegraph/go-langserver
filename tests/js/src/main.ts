import Connect from './connect';

// execute all tests - they are promise-based
let testResults = ([
    Connect
]).map(test => test());

// wait for all promises to complete or an error occurs
Promise.all(testResults).then(results => {

}).catch(excep => {

});
