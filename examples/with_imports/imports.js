import http from 'k6/http';
import {sleep} from 'k6';
import {
  randomString
} from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import {doStuff} from "./lib";
import data from './test.json' assert { type: 'json' };

export const options = {
    // A number specifying the number of VUs to run concurrently.
    vus: 10, // A string specifying the total duration of the test run.
    duration: '30s',
    tags: {
        testid: `to-do-srv-liveness`,
    },
};

const host = __ENV.TARGET || 'http://127.0.0.1:5004';

export function setup(){
    console.log(`Testing included stuff: ${host}`);
    console.log(`Data: ${JSON.stringify(data)}`);
}
// The function that defines VU logic.
//
// See https://grafana.com/docs/k6/latest/examples/get-started-with-k6/ to learn more
// about authoring k6 scripts.
//
export default function () {
    randomString(10);
    doStuff();


    http.get(`${host}/liveness`);
    sleep(1);
}
