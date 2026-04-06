/**
 * @param {string} method
 * @param {string} url
 * @param {{ body?: BodyInit|FormData|File|Record<string, unknown>|null, headers?: HeadersInit }} [options]
 */
export function request(method, url, { body = undefined, headers = undefined } = {}) {
    let payload = body
    if (
        !(payload instanceof FormData) &&
        !(payload instanceof File) &&
        typeof payload === "object" &&
        payload !== null
    ) {
        payload = JSON.stringify(payload)
    }
    return fetch(url, {
        method,
        headers: Object.assign({}, headers),
        credentials: "include",
        body: payload,
    }).then(handleResponse)
}

/**
 * @param {string} url
 * @param {(data: unknown) => void} cb
 */
export function subscribe(url, cb) {
    /** @param {MessageEvent<string>} ev */
    const onMessage = ev => {
        try {
            const data = JSON.parse(ev.data)
            cb(data)
        } catch (_) {}
    }

    const noop = () => {}

    const es = new EventSource(url, { withCredentials: true })
    es.addEventListener("message", onMessage)
    es.addEventListener("error", noop)

    return () => {
        es.removeEventListener("message", onMessage)
        es.removeEventListener("error", noop)
        es.close()
    }
}

/**
 * @param {Response} resp
 */
export function handleResponse(resp) {
    return resp
        .clone()
        .json()
        .catch(() => resp.text())
        .then(body => {
            if (!resp.ok) {
                const err = new Error()
                if (typeof body === "string" && body.trim() !== "") {
                    err.message = body.trim()
                } else if (typeof body === "object" && body !== null && typeof body.error === "string") {
                    err.message = body.error
                } else {
                    err.message = resp.statusText
                }
                err.name = err.message
                    .split(" ")
                    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
                    .join("")
                if (!err.name.endsWith("Error")) {
                    err.name = err.name + "Error"
                }
                const responseErr = /** @type {Error & { headers?: Headers, statusCode?: number }} */ (err)
                responseErr.headers = resp.headers
                responseErr.statusCode = resp.status
                throw err
            }
            return {
                body,
                headers: resp.headers,
                statusCode: resp.status,
            }
        })
}
