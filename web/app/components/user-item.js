import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { ifDefined } from "lit/directives/if-defined.js"
import { authStore, useStore } from "../ctx.js"
import { Avatar } from "./avatar.js"
import "./user-follow-btn.js"
import "./user-follow-counts.js"

function UserItem({ user: initialUser }) {
    const [auth] = useStore(authStore)
    const [user, setUser] = useState(initialUser)

    const onFollowToggle = ev => {
        const payload = ev.detail
        setUser(u => ({
            ...u,
            ...payload,
        }))
    }

    useEffect(() => {
        setUser(initialUser)
    }, [initialUser])

    return html`
        <article class="user-item" style="${ifDefined(user.coverURL !== null ? `--cover-url: url('${user.coverURL}');` : undefined)}">
            <a href="/@${user.username}" class="user-info">
                ${Avatar(user)}
                <div class="user-text">
                    <div class="username-row">
                        <span class="username">${user.username}</span>
                        ${user.followsViewer ? html`
                            <span class="follows-viewer-badge">follows you</span>
                        ` : null}
                    </div>
                    <user-follow-counts .user=${user}></user-follow-counts>
                </div>
            </a>
            ${auth !== null && !user.isMe ? html`
                <user-follow-btn .user=${user} @follow-toggle=${onFollowToggle}></user-follow-btn>
            ` : null}
        </article>
    `
}

// @ts-ignore
customElements.define("user-item", component(UserItem, { useShadowDOM: false }))
