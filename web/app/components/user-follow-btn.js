import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { ifDefined } from "lit/directives/if-defined.js"
import { request } from "../http.js"
import "./toast-item.js"

function UserFollowBtn({ user: initialUser }) {
    const [user, setUser] = useState(initialUser)
    const [fetching, setFetching] = useState(false)
    const [toast, setToast] = useState(null)

    const dispatchFollowToggle = payload => {
        this.dispatchEvent(new CustomEvent("follow-toggle", {
            bubbles: true,
            detail: payload,
        }))
    }

    const onClick = () => {
        setFetching(true)
        toggleFollow(user.username).then(payload => {
            setUser(u => ({ ...u, ...payload }))
            dispatchFollowToggle(payload)
        }, err => {
            const msg = "could not toggle follow: " + err.message
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setFetching(false)
        })
    }

    useEffect(() => {
        setUser(initialUser)
    }, [initialUser])

    if (user.isMe) {
        return null
    }

    return html`
        <button aria-busy=${ifDefined(fetching ? "true" : undefined)} .disabled=${fetching} @click=${onClick}>
            ${user.followedByViewer ? html`
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="person-done"><rect width="24" height="24" opacity="0"/><path d="M21.66 4.25a1 1 0 0 0-1.41.09l-1.87 2.15-.63-.71a1 1 0 0 0-1.5 1.33l1.39 1.56a1 1 0 0 0 .75.33 1 1 0 0 0 .74-.34l2.61-3a1 1 0 0 0-.08-1.41z"/><path d="M10 11a4 4 0 1 0-4-4 4 4 0 0 0 4 4zm0-6a2 2 0 1 1-2 2 2 2 0 0 1 2-2z"/><path d="M10 13a7 7 0 0 0-7 7 1 1 0 0 0 2 0 5 5 0 0 1 10 0 1 1 0 0 0 2 0 7 7 0 0 0-7-7z"/></g></g></svg>
            ` : html`
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="person-add"><rect width="24" height="24" opacity="0"/><path d="M21 6h-1V5a1 1 0 0 0-2 0v1h-1a1 1 0 0 0 0 2h1v1a1 1 0 0 0 2 0V8h1a1 1 0 0 0 0-2z"/><path d="M10 11a4 4 0 1 0-4-4 4 4 0 0 0 4 4zm0-6a2 2 0 1 1-2 2 2 2 0 0 1 2-2z"/><path d="M10 13a7 7 0 0 0-7 7 1 1 0 0 0 2 0 5 5 0 0 1 10 0 1 1 0 0 0 2 0 7 7 0 0 0-7-7z"/></g></g></svg>
            `}
            <span>${user.followedByViewer ? "Following" : "Follow"}</span>
        </button>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

// @ts-ignore
customElements.define("user-follow-btn", component(UserFollowBtn, { useShadowDOM: false }))

/**
 * @param {string} username
 */
function toggleFollow(username) {
    return request("POST", `/api/users/${encodeURIComponent(username)}/toggle_follow`)
        .then(resp => resp.body)
}
