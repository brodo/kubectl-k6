import {check, sleep} from 'k6';
import http from 'k6/http';
import {Counter, Trend} from 'k6/metrics';
import {vu} from 'k6/execution';

const url = __ENV.TARGET;
const api_version = '/topologymanagement/api/v6/';

export const TrendRTT = new Trend('RTT');
export const CounterErrors = new Counter('Errors');
export const CounterSuccess = new Counter('Success');

export const options = {
    thresholds: {
        'Errors': ['count<100'],
        'Success': ['count>100'],
        'RTT': [
            'p(95)<350',
            'p(90)<300',
            'avg<200',
        ],
    },
    scenarios: {
        Minimal_Load: {
            executor: 'constant-vus',
            duration: '2m',
            vus: 1,
            gracefulStop: '5s',
            tags: {
                'testId': `K6K8S-GetNodesEndpointMinimalLoad-${new Date().toISOString()}`,
                'Api': api_version,
            },
        },
    },
};

export function setup() {
    const credentials = [
        {
            clientId: 'moneo-management@cpp',
            clientSecret: 'euUT2CemWm4TL9CtRrD5dQgYuhfxrGwsyGqazUvpYyZk'
        },
        {
            clientId: 'moneo-management@cpp',
            clientSecret: 'euUT2CemWm4TL9CtRrD5dQgYuhfxrGwsyGqazUvpYyZk'
        },
        {
            clientId: 'moneo-management@cpp',
            clientSecret: 'euUT2CemWm4TL9CtRrD5dQgYuhfxrGwsyGqazUvpYyZk'
        },
    ];
    const tokens = [];
    for (let c of credentials) {
        tokens.push(getAuthToken(url, c.clientId, c.clientSecret));
        console.log("added token: ", tokens[tokens.length - 1])
    }

    return {tokens};
}


function getAuthToken(baseUrl, clientId, clientSecret) {
    const response = http.post(`${baseUrl}/authorizationprovider/connect/token`,
        {
            grant_type: 'client_credentials',
            client_id: clientId,
            client_secret: clientSecret,
            response_type: 'token'
        }
    );
    check(response, {'authentication status code 200': (r) => r.status === 200});
    return response.json().access_token;
}


export default function ({tokens}) {
    // VU identifiers are one-based and arrays are zero-based, thus we need - 1
    const token = tokens[(vu.idInTest - 1) % tokens.length];
    let response = http.get(`${url}${api_version}nodes?topologies=20`,
        {headers: {authorization: `Bearer ${token}`}});

    TrendRTT.add(response.timings.duration);

    CounterSuccess.add(response.status === 200);

    if (response.status !== 200 && response.status !== 404) {
        CounterErrors.add(response.status);
    }
    sleep(1)
}
