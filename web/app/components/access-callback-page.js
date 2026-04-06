import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import {
    applyAuth,
    clearAuth,
    completeSignup,
    errorFromMessage,
    loadAuthFromSession,
    renderPendingSignupForm,
} from "../lib/access-auth.js"
import { authStore, useStore } from "../ctx.js"

export default function () {
    return html`<access-callback-page></access-callback-page>`
}

function AccessCallbackPage() {
    const [, setAuth] = useStore(authStore)
    const [err, setErr] = useState(/** @type {Error|null} */ (null))
    const [pendingSignup, setPendingSignup] = useState(false)
    const [fetching, setFetching] = useState(false)

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
            .catch(err => {
                setErr(err)
            })
            .finally(() => {
                setFetching(false)
            })
    }

    useEffect(() => {
        const data = new URLSearchParams(location.hash.substr(1))
        const result = data.get("result")

        if (result === "error") {
            setErr(errorFromMessage(data.get("error") ?? "unknown error"))
            setPendingSignup(false)
            clearAuth(setAuth)
            return
        }

        if (result === "pending_signup") {
            setErr(null)
            setPendingSignup(true)
            clearAuth(setAuth)
            return
        }

        if (result === "success") {
            setErr(null)
            setPendingSignup(false)
            loadAuthFromSession().then(
                auth => {
                    applyAuth(auth, setAuth)
                },
                err => {
                    setErr(err)
                    clearAuth(setAuth)
                },
            )
            return
        }

        setErr(errorFromMessage("missing auth result"))
        setPendingSignup(false)
    }, [])

    return html`
        <main class="container access-callback-page">
            <section class="access-callback-panel">
                <p class="access-callback-kicker">
                    ${pendingSignup ? "Almost there" : err !== null ? "Sign-in problem" : "Signing you in"}
                </p>
                <h1>
                    ${pendingSignup
                        ? "Choose your username"
                        : err !== null
                          ? "We couldn’t finish sign in"
                          : "Finishing sign in"}
                </h1>
                <p class="access-callback-summary">
                    ${pendingSignup
                        ? "Your social sign-in is waiting for a username. Pick a public username to create your Nakama account and complete signup."
                        : err !== null
                          ? "Something interrupted the authentication flow. You can go back home and try again."
                          : fetching
                            ? "Creating your account and starting your session..."
                            : "Checking your session and preparing your account..."}
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

                          ${err !== null ? html`<p class="error" role="alert">${err.message}</p>` : null}
                          ${renderPendingSignupForm({ err, fetching, onSubmit: onUsernameFormSubmit })}
                      `
                    : null}
                ${!pendingSignup && err !== null
                    ? html`<a class="btn access-callback-home" href="/">Go home</a>`
                    : null}
                ${!pendingSignup && err === null
                    ? html`<p class="loader" aria-busy="true" aria-live="polite">Please wait...</p>`
                    : null}
            </section>
        </main>
    `
}

customElements.define("access-callback-page", component(AccessCallbackPage, { useShadowDOM: false }))
