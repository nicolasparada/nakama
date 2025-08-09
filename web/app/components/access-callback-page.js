import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { translate } from "lit-translate"
import { setLocalAuth } from "../auth.js"
import { authStore, useStore } from "../ctx.js"
import { navigate } from "../router.js"

export default function () {
    return html`<access-callback-page></access-callback-page>`
}

function AccessCallbackPage() {
    const [, setAuth] = useStore(authStore)
    const [err, setErr] = useState(/** @type {Error|null} */(null))
    const [retryEndpoint, setRetryEndpoint] = useState(/** @type {URL|null} */(null))
    const [username, setUsername] = useState("")

    const onUsernameFormSubmit = ev => {
        ev.preventDefault()

        if (retryEndpoint === null) {
            return
        }

        retryEndpoint.searchParams.set("username", username)
        location.replace(retryEndpoint.toString())
    }

    const onUsernameInput = ev => {
        setUsername(ev.currentTarget.value)
    }

    useEffect(() => {
        const data = new URLSearchParams(location.hash.substr(1))
        if (data.has("error")) {
            const err = new Error(decodeURIComponent(data.get("error")))
            err.name = err.message
                .split(" ")
                .map(word => word.charAt(0).toUpperCase() + word.slice(1))
                .join("")
                + "Error"

            setErr(err)

            setLocalAuth(null)
            setAuth(null)
            
            if (data.has("retry_endpoint") && isRetriableError(err)) {
                setRetryEndpoint(new URL(decodeURIComponent(data.get("retry_endpoint")), location.origin))
                return
            }
            
            return
        }

        if (!data.has("token") || !data.has("expires_at") || !data.has("user.id") || !data.has("user.username")) {
            const err = new Error("missing auth data")
            err.name = "MissingAuthDataError"
            setErr(err)
            return
        }

        const auth = {
            token: decodeURIComponent(data.get("token")),
            expiresAt: new Date(decodeURIComponent(data.get("expires_at"))),
            user: {
                id: decodeURIComponent(data.get("user.id")),
                username: decodeURIComponent(data.get("user.username")),
                avatarURL: data.has("user.avatar_url") ? decodeURIComponent(data.get("user.avatar_url")) : null,
            }
        }

        setLocalAuth(auth)
        setAuth(auth)
        navigate("/", true)
    }, [])

    return html`
        <main class="container">
            <h1>${translate("accessCallbackPage.title")}</h1>
            ${err !== null ? html`
                <p class="error" role="alert">${translate("accessCallbackPage.err")} ${translate(err.name)}</p>
                ${!isRetriableError(err) ? html`
                    <a href="/">${translate("accessCallbackPage.goHome")}</a>
                ` : null}
            ` : null}
            ${retryEndpoint !== null ? html`
                <form class="username-form" @submit=${onUsernameFormSubmit}>
                    <input type="text" name="username" placeholder="${translate("accessCallbackPage.usernamePlaceholder")}" pattern="^[a-zA-Z][a-zA-Z0-9_-]{0,17}$" autofocus .value=${username} @input=${onUsernameInput}>
                    <button>${translate("accessCallbackPage.createAccountBtn")}</button>
                </form>
            ` : null}
        </main>
    `
}

customElements.define("access-callback-page", component(AccessCallbackPage, { useShadowDOM: false }))

function isRetriableError(err) {
    return err.name === "UserNotFoundError" || err.name === "InvalidUsernameError" || err.name === "UsernameTakenError"
}
