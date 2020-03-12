import 'whatwg-fetch';

require('es6-promise').polyfill();

const headers = {
    'X-Requested-With': 'XMLHttpRequest',
    Accept: 'application/json',
};
const cache = {};
const jsonHeaders = ['application/json', 'application/problem+json'];

function isJson(response) {
    return jsonHeaders.indexOf(response.headers.get('content-type')) >= 0
}

function checkStatus(response) {
    let retval;

    if (isJson(response) && response.status !== 204 && response.status !== 429) {
        retval = response.text().then((text) => {
            let data;

            try {
                data = JSON.parse(text);
            } catch (e) {
                data = text;
            }

            return {
                status: response.status,
                statusText: response.statusText,
                data,
                response,
            };
        });
    } else {
        retval = response.text().then((text) => ({
            status: response.status,
            statusText: response.statusText,
            data: text,
            response,
        }));
    }

    if (response.status === 429) {
        retval.then((retvalData) => {
            retvalData.data = {
                code: 429,
                message: 'Rate limit exceeded',
                errors: null,
            };

            return retvalData;
        });
    }

    if (response.status >= 200 && response.status < 300) {
        return retval;
    }

    return retval.then((retvalData) => {
        const error = new Error(retvalData.statusText);

        error.response = retvalData;

        throw error;
    });
}

export default function doFetch(url, method = 'GET', auth = {}, data = null, customHeaders = {}, progressCallback = null) {
    const body = {};
    const bodyHeaders = {};

    if (data !== null) {
        if (typeof data === 'object') {
            body.body = JSON.stringify(data);
            bodyHeaders['content-type'] = 'application/json';
        } else if (data) {
            body.body = data;
        }
    }

    function result() {
        const options = {
            method,
            headers: {
                ...headers, ...bodyHeaders, ...auth, ...customHeaders,
            },
            ...body,
        };
        const key = `${JSON.stringify(options)}@@@${url}`;

        // this is not an actual cache but an anti concurrent mitigation system
        if (cache[key] !== undefined) {
            return cache[key];
        }

        async function fetchResult() {
            const response = await fetch(url, options);
            const finalResponse = response.clone();

            if (!response.body || !response.body.getReader) {
                return finalResponse;
            }

            const reader = response.body.getReader();
            const contentLength = +response.headers.get('X-Blackfire-Content-Length');

            if (!progressCallback || contentLength === 0) {
                return finalResponse;
            }

            let receivedLength = 0;
            /*eslint-disable no-constant-condition*/
            while (true) {
            /*eslint-enable no-constant-condition*/
                /*eslint-disable no-await-in-loop*/
                const { done, value } = await reader.read();
                /*eslint-enable no-await-in-loop*/

                if (done) {
                    break;
                }

                receivedLength += value.length;
                progressCallback(receivedLength, contentLength);
            }

            return finalResponse;
        }

        const ret = fetchResult()
            .then(checkStatus)
            .then((response) => {
                delete cache[key];

                return response;
            }, (error) => {
                delete cache[key];

                throw error;
            });

        cache[key] = ret;

        return ret;
    }

    return result();
}
