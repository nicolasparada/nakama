import { component, useState } from "haunted"
import { html } from "lit"
import { get as getTranslation, translate } from "lit-translate"
import { authFromUser, setLocalAuth } from "../auth.js"
import { authStore, useStore } from "../ctx.js"
import { request } from "../http.js"
import { navigate } from "../router.js"
import "./toast-item.js"

/** @typedef {import("./toast-item.js").Toast} Toast */
/** @typedef {import("../types.js").AuthState} AuthState */

const inLocalhost = ["127.0.0.1", "localhost"].some(s => s === location.hostname) && !Boolean(import.meta.env.PROD)

function isDevLoginEnabled() {
    if (!inLocalhost) {
        return false
    }

    const dev = new URLSearchParams(location.search).get("dev")
    if (dev === null) {
        return true
    }

    return !["0", "false", "no", "off"].includes(dev.trim().toLowerCase())
}

export default function AccessPage() {
    return html`
        <main class="container access-page">
            <h1>${translate("accessPage.title")}</h1>
            <p>${translate("accessPage.welcome")}</p>
            <login-form></login-form>
        </main>
    `
}

function LoginForm() {
    const devLoginEnabled = isDevLoginEnabled()
    const [, setAuth] = useStore(authStore)
    const [email, setEmail] = useState(devLoginEnabled ? "shinji@example.org" : "")
    const [fetching, setFetching] = useState(false)
    const [toast, setToast] = useState(/** @type {Toast|null} */ (null))

    /** @param {SubmitEvent} ev */
    const onSubmit = ev => {
        ev.preventDefault()

        setFetching(true)

        /** @param {Error} err */
        const onErr = err => {
            const msg =
                (devLoginEnabled
                    ? getTranslation("loginForm.errLogin")
                    : getTranslation("loginForm.errSendMagicLink")) + getTranslation(err.name)
            console.error(msg)
            setToast({ type: "error", content: msg })
        }

        if (devLoginEnabled) {
            devLogin(email)
                .then(auth => {
                    setEmail("")
                    setLocalAuth(auth)
                    setAuth(auth)
                }, onErr)
                .finally(() => {
                    setFetching(false)
                })
            return
        }

        requestLogin(email)
            .then(() => {
                navigate("/access-login?requested=true")
            }, onErr)
            .finally(() => {
                setFetching(false)
            })
    }

    /** @param {Event & { currentTarget: HTMLInputElement }} ev */
    const onEmailInput = ev => {
        setEmail(ev.currentTarget.value)
    }

    return html`
        <form class="access-form" @submit=${onSubmit}>
            <input
                id="email-input"
                type="email"
                autocomplete="email"
                required
                placeholder="${translate("loginForm.placeholder")}"
                .value=${email}
                .disabled=${fetching}
                @input=${onEmailInput}
            />
            <button .disabled=${fetching}>${translate(devLoginEnabled ? "loginForm.devBtn" : "loginForm.btn")}</button>
        </form>
        <h3>${translate("loginForm.subheading")}</h3>
        <div class="oauth-providers">
            <a
                class="btn"
                href="${location.origin}/api/auth/github/redirect?redirect_uri=${encodeURIComponent(
                    location.origin + "/access-callback",
                )}"
                data-default="true"
            >
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                    <g data-name="Layer 2">
                        <rect width="24" height="24" opacity="0" />
                        <path
                            d="M16.24 22a1 1 0 0 1-1-1v-2.6a2.15 2.15 0 0 0-.54-1.66 1 1 0 0 1 .61-1.67C17.75 14.78 20 14 20 9.77a4 4 0 0 0-.67-2.22 2.75 2.75 0 0 1-.41-2.06 3.71 3.71 0 0 0 0-1.41 7.65 7.65 0 0 0-2.09 1.09 1 1 0 0 1-.84.15 10.15 10.15 0 0 0-5.52 0 1 1 0 0 1-.84-.15 7.4 7.4 0 0 0-2.11-1.09 3.52 3.52 0 0 0 0 1.41 2.84 2.84 0 0 1-.43 2.08 4.07 4.07 0 0 0-.67 2.23c0 3.89 1.88 4.93 4.7 5.29a1 1 0 0 1 .82.66 1 1 0 0 1-.21 1 2.06 2.06 0 0 0-.55 1.56V21a1 1 0 0 1-2 0v-.57a6 6 0 0 1-5.27-2.09 3.9 3.9 0 0 0-1.16-.88 1 1 0 1 1 .5-1.94 4.93 4.93 0 0 1 2 1.36c1 1 2 1.88 3.9 1.52a3.89 3.89 0 0 1 .23-1.58c-2.06-.52-5-2-5-7a6 6 0 0 1 1-3.33.85.85 0 0 0 .13-.62 5.69 5.69 0 0 1 .33-3.21 1 1 0 0 1 .63-.57c.34-.1 1.56-.3 3.87 1.2a12.16 12.16 0 0 1 5.69 0c2.31-1.5 3.53-1.31 3.86-1.2a1 1 0 0 1 .63.57 5.71 5.71 0 0 1 .33 3.22.75.75 0 0 0 .11.57 6 6 0 0 1 1 3.34c0 5.07-2.92 6.54-5 7a4.28 4.28 0 0 1 .22 1.67V21a1 1 0 0 1-.94 1z"
                        />
                    </g>
                </svg>
                <span>GitHub</span>
            </a>
            <a
                class="btn"
                href="${location.origin}/api/auth/google/redirect?redirect_uri=${encodeURIComponent(
                    location.origin + "/access-callback",
                )}"
                data-default="true"
            >
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                    <g data-name="Layer 2">
                        <g data-name="google">
                            <polyline points="0 0 24 0 24 24 0 24" opacity="0" />
                            <path
                                d="M12 22h-.43A10.16 10.16 0 0 1 2 12.29a10 10 0 0 1 14.12-9.41 1.48 1.48 0 0 1 .77.86 1.47 1.47 0 0 1-.1 1.16L15.5 7.28a1.44 1.44 0 0 1-1.83.64A4.5 4.5 0 0 0 8.77 9a4.41 4.41 0 0 0-1.16 3.34 4.36 4.36 0 0 0 1.66 3 4.52 4.52 0 0 0 3.45 1 3.89 3.89 0 0 0 2.63-1.57h-2.9A1.45 1.45 0 0 1 11 13.33v-2.68a1.45 1.45 0 0 1 1.45-1.45h8.1A1.46 1.46 0 0 1 22 10.64v1.88A10 10 0 0 1 12 22zm0-18a8 8 0 0 0-8 8.24A8.12 8.12 0 0 0 11.65 20 8 8 0 0 0 20 12.42V11.2h-7v1.58h5.31l-.41 1.3a6 6 0 0 1-4.9 4.25A6.58 6.58 0 0 1 8 17a6.33 6.33 0 0 1-.72-9.3A6.52 6.52 0 0 1 14 5.91l.77-1.43A7.9 7.9 0 0 0 12 4z"
                            />
                        </g>
                    </g>
                </svg>
                <span>Google</span>
            </a>
            <a
                class="btn"
                href="${location.origin}/api/auth/discord/redirect?redirect_uri=${encodeURIComponent(
                    location.origin + "/access-callback",
                )}"
                data-default="true"
            >
                <svg
                    xmlns="http://www.w3.org/2000/svg"
                    viewBox="0 0 24 24"
                    style="fill: none; stroke: var(--icon); stroke-width: 2; stroke-linecap: round; stroke-linejoin: round;"
                >
                    <path d="M8 12a1 1 0 1 0 2 0a1 1 0 0 0-2 0" />
                    <path d="M14 12a1 1 0 1 0 2 0a1 1 0 0 0-2 0" />
                    <path
                        d="M15.5 17c0 1 1.5 3 2 3c1.5 0 2.833-1.667 3.5-3c.667-1.667.5-5.833-1.5-11.5c-1.457-1.015-3-1.34-4.5-1.5l-.972 1.923a11.913 11.913 0 0 0-4.053 0L9 4c-1.5.16-3.043.485-4.5 1.5c-2 5.667-2.167 9.833-1.5 11.5c.667 1.333 2 3 3.5 3c.5 0 2-2 2-3"
                    />
                    <path d="M7 16.5c3.5 1 6.5 1 10 0" />
                </svg>
                <span>Discord</span>
            </a>
        </div>
        <div class="access-help">
            <div>
                <h3>${translate("loginForm.signupHelp.heading")}</h3>
                <p>${translate("loginForm.signupHelp.summary")}</p>
            </div>
            <div>
                <h3>${translate("loginForm.signinHelp.heading")}</h3>
                <p>${translate("loginForm.signinHelp.summary")}</p>
            </div>
        </div>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

customElements.define("login-form", component(LoginForm, { useShadowDOM: false }))

/**
 * @param {string} email
 * @returns {Promise<AuthState>}
 */
function devLogin(email) {
    return request("POST", "/api/dev_login", { body: { email } }).then(resp => authFromUser(resp.body))
}

/**
 * @param {string} email
 * @param {string} [redirectURI]
 */
function requestLogin(email, redirectURI = location.origin + "/access-login") {
    return request("POST", "/api/auth/login/request", { body: { email, redirectURI } })
}
