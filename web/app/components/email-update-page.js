import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { errorFromMessage, verifyEmailUpdate } from "../lib/access-auth.js"
import { navigate } from "../router.js"

export default function () {
    return html`<email-update-page></email-update-page>`
}

function EmailUpdatePage() {
    const [err, setErr] = useState(/** @type {Error|null} */ (null))
    const [fetching, setFetching] = useState(false)

    useEffect(() => {
        const query = new URLSearchParams(location.search)
        const code = query.get("code")

        if (code === null || code.trim() === "") {
            setErr(errorFromMessage("missing email verification code"))
            return
        }

        setFetching(true)
        setErr(null)

        verifyEmailUpdate(code)
            .then(user => {
                navigate(`/@${encodeURIComponent(user.username)}`, true)
            })
            .catch(verifyErr => {
                setErr(verifyErr)
            })
            .finally(() => {
                setFetching(false)
            })
    }, [])

    return html`
        <main class="container access-callback-page">
            <section class="access-callback-panel">
                <p class="access-callback-kicker">${err !== null ? "Email update problem" : "Updating your email"}</p>
                <h1>${err !== null ? "We couldn’t update your email" : "Verifying your new email"}</h1>
                <p class="access-callback-summary">
                    ${err !== null
                        ? "Something interrupted the email verification flow. Make sure you are still signed in and try opening the verification link again."
                        : fetching
                          ? "Verifying your new email address and updating your account..."
                          : "Checking your verification link..."}
                </p>
                ${err !== null ? html`<a class="btn access-callback-home" href="/">Go home</a>` : null}
                ${err === null ? html`<p class="loader" aria-busy="true" aria-live="polite">Please wait...</p>` : null}
            </section>
        </main>
    `
}

customElements.define("email-update-page", component(EmailUpdatePage, { useShadowDOM: false }))
