import http from 'k6/http';
import {sleep} from 'k6';


export const options = {
    scenarios: {
        min_load: {
            executor: 'constant-vus',
            vus:1,
            duration: '2m',
            tags: {
                'testId': `ToDoApp-Liveness-${new Date().toISOString()}`,
            }
        },
    },

};

const host = __ENV.TARGET || 'http://127.0.0.1:5004';

export function setup(){
    console.log(`Checking liveness endpoint: ${host}`);
}
// The function that defines VU logic.
//
// See https://grafana.com/docs/k6/latest/examples/get-started-with-k6/ to learn more
// about authoring k6 scripts.
//
export default function () {
    http.get(`${host}/liveness`);
    sleep(1);
}
