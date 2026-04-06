import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import {
    applyAuth,
    clearAuth,
    completeSignup,
    errorFromMessage,
    loadAuthFromSession,
    renderPendingSignupForm,
    verifyPasswordlessLogin,
} from "../lib/access-auth.js"
import { authStore, useStore } from "../ctx.js"

/** @typedef {import("../types.js").LoginResult} LoginResult */

export default function () {
    return html`<access-login-page></access-login-page>`
}

function AccessLoginPage() {
    const [, setAuth] = useStore(authStore)
    const [err, setErr] = useState(/** @type {Error|null} */ (null))
    const [code, setCode] = useState("")
    const [prefilledCode, setPrefilledCode] = useState(false)
    const [pendingSignup, setPendingSignup] = useState(false)
    const [fetching, setFetching] = useState(false)
    const [requested, setRequested] = useState(false)

    /** @type {(result: LoginResult) => Promise<void>} */
    const handleLoginResult = result => {
        if (result.status === "pending_signup") {
            setPendingSignup(true)
            clearAuth(setAuth)
            return Promise.resolve()
        }

        setPendingSignup(false)
        return loadAuthFromSession().then(
            auth => {
                applyAuth(auth, setAuth)
            },
            loadErr => {
                setErr(loadErr)
                clearAuth(setAuth)
            },
        )
    }

    /** @param {SubmitEvent} ev */
    const onCodeFormSubmit = ev => {
        ev.preventDefault()

        if (fetching) {
            return
        }

        const trimmedCode = code.trim()
        if (trimmedCode === "") {
            setErr(errorFromMessage("missing login code"))
            return
        }

        setFetching(true)
        setErr(null)

        verifyPasswordlessLogin(trimmedCode)
            .then(handleLoginResult)
            .catch(verifyErr => {
                setErr(verifyErr)
                setPendingSignup(false)
                clearAuth(setAuth)
            })
            .finally(() => {
                setFetching(false)
            })
    }

    /** @param {Event & { currentTarget: HTMLInputElement }} ev */
    const onCodeInput = ev => {
        setCode(ev.currentTarget.value)
    }

    /** @param {SubmitEvent} ev */
    const onUsernameFormSubmit = ev => {
        ev.preventDefault()

        if (!(ev.currentTarget instanceof HTMLFormElement)) {
            return
        }

        if (fetching) {
            return
        }

        const formData = new FormData(ev.currentTarget)
        const username = formData.get("username")
        if (typeof username !== "string") {
            setErr(errorFromMessage("username is required"))
            return
        }

        setFetching(true)
        setErr(null)

        completeSignup(username)
            .then(auth => {
                applyAuth(auth, setAuth)
            })
            .catch(submitErr => {
                setErr(submitErr)
            })
            .finally(() => {
                setFetching(false)
            })
    }

    useEffect(() => {
        const params = new URLSearchParams(location.search)
        const initialCode = params.get("code")
        const hasPrefilledCode = initialCode !== null && initialCode.trim() !== ""
        setCode(initialCode === null ? "" : initialCode)
        setPrefilledCode(hasPrefilledCode)
        setRequested(params.get("requested") === "true")
        setErr(null)
        setPendingSignup(false)
    }, [])

    return html`
        <main class="container access-callback-page">
            <section class="access-callback-panel">
                <p class="access-callback-kicker">
                    ${pendingSignup
                        ? "Almost there"
                        : err !== null
                          ? "Sign-in problem"
                          : prefilledCode
                            ? "Ready to sign in"
                            : code.trim() !== ""
                              ? "Code ready"
                              : requested
                                ? "Check your inbox"
                                : "Enter login code"}
                </p>
                <h1>
                    ${pendingSignup
                        ? "Choose your username"
                        : err !== null
                          ? "We couldn’t finish sign in"
                          : "Sign in with your login code"}
                </h1>
                <p class="access-callback-summary">
                    ${pendingSignup
                        ? "Your email sign-in is waiting for a username. Pick a public username to create your Nakama account and complete signup."
                        : err !== null
                          ? "Something interrupted the authentication flow. You can go back home and try again."
                          : prefilledCode
                            ? "Your login code was filled in from the email link. Click Login to verify it and start your session."
                            : requested
                              ? "We sent you a magic link and a login code. Paste the code below, or open the email link to prefill the form and then click verify."
                              : code.trim() !== ""
                                ? "The login code from your email has been prefilled. Review it if needed, then click verify to continue."
                                : "Paste the login code from your email, or open the magic link and this form will prefill it for you."}
                </p>

                ${pendingSignup
                    ? html`
                          <div class="access-callback-hints">
                              <p class="access-callback-hints-title">Before you continue:</p>
                              <ul>
                                  <li>Your username will appear on your profile and posts.</li>
                                  <li>It must start with a letter.</li>
                                  <li>You can use letters, numbers, underscores, and hyphens.</li>
                              </ul>
                          </div>

                          ${renderPendingSignupForm({ err, fetching, onSubmit: onUsernameFormSubmit })}
                      `
                    : html`
                          ${err !== null ? html`<p class="error" role="alert">${err.message}</p>` : null}
                          <form class="access-callback-form" @submit=${onCodeFormSubmit}>
                              <label class="access-callback-label" for="login-code-input">Login code</label>
                              <input
                                  class="access-login-code-input"
                                  id="login-code-input"
                                  type="text"
                                  name="code"
                                  autocomplete="off"
                                  autocapitalize="off"
                                  spellcheck="false"
                                  placeholder=${prefilledCode ? "Code loaded from email link" : "Paste your login code"}
                                  .value=${code}
                                  .disabled=${fetching || prefilledCode}
                                  @input=${onCodeInput}
                              />
                              <p class="access-callback-help">
                                  ${prefilledCode
                                      ? "Your code came from the email link. Click Login to continue."
                                      : "Copy the code from the email, or use the email link and then click Login."}
                              </p>
                              <div class="access-callback-actions">
                                  <button .disabled=${fetching || code.trim() === ""}>
                                      ${fetching ? "Logging in..." : "Login"}
                                  </button>
                              </div>
                          </form>
                      `}
            </section>
        </main>
    `
}

customElements.define("access-login-page", component(AccessLoginPage, { useShadowDOM: false }))
