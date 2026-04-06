import { html } from "lit"
import { authFromUser, setLocalAuth } from "../auth.js"
import { request } from "../http.js"
import { navigate } from "../router.js"

/** @typedef {import("../types.js").AuthState} AuthState */

/**
 * @param {string} message
 * @returns {Error}
 */
export function errorFromMessage(message) {
    const err = new Error(message)
    err.name =
        message
            .split(" ")
            /** @param {string} word */
            .map(word => word.charAt(0).toUpperCase() + word.slice(1))
            .join("") + "Error"
    return err
}

/**
 * @param {string} username
 * @returns {Promise<AuthState>}
 */
export function completeSignup(username) {
    return request("POST", "/api/auth/signup/complete", {
        body: {
            username: username.trim(),
        },
    }).then(resp => authFromUser(resp.body))
}

/** @returns {Promise<AuthState>} */
export function loadAuthFromSession() {
    return request("GET", "/api/user").then(resp => authFromUser(resp.body))
}

/**
 * @param {string} code
 * @returns {Promise<import("../types.js").LoginResult>}
 */
export function verifyPasswordlessLogin(code) {
    return request("POST", "/api/auth/login/verify", {
        body: {
            code: code.trim(),
        },
    }).then(resp => normalizeLoginResult(resp.body))
}

/**
 * @param {string} code
 * @returns {Promise<import("../types.js").User>}
 */
export function verifyEmailUpdate(code) {
    return request("PATCH", "/api/user/email/verify", {
        body: {
            code: code.trim(),
        },
    }).then(resp => normalizeUser(resp.body))
}

/**
 * @param {AuthState} auth
 * @param {(nextAuth: AuthState|null) => void} setAuth
 */
export function applyAuth(auth, setAuth) {
    setLocalAuth(auth)
    setAuth(auth)
    navigate("/", true)
}

/** @param {(nextAuth: AuthState|null) => void} setAuth */
export function clearAuth(setAuth) {
    setLocalAuth(null)
    setAuth(null)
}

/**
 * @param {{ err: Error|null, fetching: boolean, onSubmit: (event: SubmitEvent) => void }} props
 */
export function renderPendingSignupForm({ err, fetching, onSubmit }) {
    return html`
        ${err !== null ? html`<p class="error" role="alert">${pendingSignupErrorMessage(err)}</p>` : null}
        <form class="username-form access-callback-form" @submit=${onSubmit}>
            <label class="access-callback-label" for="username-input">Username</label>
            <input
                id="username-input"
                type="text"
                name="username"
                autocomplete="username"
                autocapitalize="off"
                spellcheck="false"
                placeholder="Choose a username"
                pattern="^[a-zA-Z][a-zA-Z0-9_-]{0,17}$"
                maxlength="18"
                autofocus
                required
                aria-describedby="username-help"
                .disabled=${fetching}
            />
            <p id="username-help" class="access-callback-help">Example: shinji_01</p>
            <div class="access-callback-actions">
                <button .disabled=${fetching}>${fetching ? "Creating account..." : "Complete signup"}</button>
                ${fetching
                    ? html`<p class="loader" aria-busy="true" aria-live="polite">This usually takes a moment.</p>`
                    : null}
            </div>
        </form>
    `
}

/**
 * @param {unknown} body
 * @returns {import("../types.js").LoginResult}
 */
function normalizeLoginResult(body) {
    if (typeof body !== "object" || body === null) {
        throw errorFromMessage("invalid login response")
    }

    const status = /** @type {{ status?: unknown }} */ (body).status
    if (status !== "success" && status !== "pending_signup") {
        throw errorFromMessage("invalid login response")
    }

    return { status }
}

/**
 * @param {unknown} body
 * @returns {import("../types.js").User}
 */
function normalizeUser(body) {
    const record = /** @type {{ id?: unknown, username?: unknown, avatarURL?: unknown }|null} */ (body)
    if (
        typeof record !== "object" ||
        record === null ||
        typeof record.id !== "string" ||
        typeof record.username !== "string" ||
        !(typeof record.avatarURL === "string" || record.avatarURL === null || typeof record.avatarURL === "undefined")
    ) {
        throw errorFromMessage("invalid user response")
    }

    /** @type {import("../types.js").User} */
    const user = {
        id: record.id,
        username: record.username,
    }

    if (typeof record.avatarURL === "string") {
        user.avatarURL = record.avatarURL
    }

    return user
}

/**
 * @param {Error} err
 * @returns {string}
 */
function pendingSignupErrorMessage(err) {
    if (err.name === "InvalidUsernameError") {
        return "That username is not valid. Start with a letter and use up to 18 letters, numbers, underscores, or hyphens."
    }

    if (err.name === "UsernameTakenError") {
        return "That username is already taken. Try another one."
    }

    return err.message
}
