import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { repeat } from "lit/directives/repeat.js"
import { request } from "../http.js"
import "./intersectable-comp.js"
import "./toast-item.js"
import "./user-item.js"

/**
 * @typedef {import("../types.js").ListFollowees} ListFollowees
 */

/**
 * @typedef {import("../types.js").UserProfile} UserProfile
 */

/**
 * @template T
 * @typedef {import("../types.js").Page<T>} Page
 */

/**
 * @typedef {import("./toast-item.js").Toast} Toast
 */

export default function ({ params }) {
    return html`<user-followees-page .username=${params.username}></user-followees-page>`
}

function UserFolloweesPage({ username }) {
    const [users, setUsers] = useState([])
    const [usersEndCursor, setUsersEndCursor] = useState(null)
    const [fetching, setFetching] = useState(true)
    const [err, setErr] = useState(null)
    const [loadingMore, setLoadingMore] = useState(false)
    const [noMoreUsers, setNoMoreUsers] = useState(false)
    const [endReached, setEndReached] = useState(false)
    const [toast, setToast] = useState(null)

    const loadMore = () => {
        if (loadingMore || noMoreUsers) {
            return
        }

        setLoadingMore(true)
        fetchFollowees({ username, pageArgs: {after: usersEndCursor}}).then(page => {
            setUsers(uu => [...uu, ...page.items])
            setUsersEndCursor(page.pageInfo.endCursor)

            if (!page.pageInfo.hasNextPage) {
                setNoMoreUsers(true)
                setEndReached(true)
            }
        }, err => {
            const msg = "could not fetch more users: " + err.message
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setLoadingMore(false)
        })
    }

    useEffect(() => {
        setFetching(true)
        fetchFollowees({ username }).then(page => {
            setUsers(page.items)
            setUsersEndCursor(page.pageInfo.endCursor)

            if (!page.pageInfo.hasNextPage) {
                setNoMoreUsers(true)
            }
        }, err => {
            console.error("could not fetch users:", err)
            setErr(err)
        }).finally(() => {
            setFetching(false)
        })
    }, [username])

    return html`
        <main class="container followees-page">
            <h1>${username}'s Followees</h1>
            ${err !== null ? html`
                <p class="error" role="alert">Could not fetch followees: ${err.message}</p>
            ` : fetching ? html`
                <p class="loader" aria-busy="true" aria-live="polite">Loading followees... please wait.</p>
            ` : html`
                ${users.length === 0 ? html`
                    <p>0 followees</p>
                ` : html`
                    <div class="users" role="feed">
                        ${repeat(users, u => u.id, u => html`<user-item .user=${u}></user-item>`)}
                    </div>
                    ${!noMoreUsers ? html`
                        <intersectable-comp @is-intersecting=${loadMore}></intersectable-comp>
                        <p class="loader" aria-busy="true" aria-live="polite">Loading users... please wait.</p>
                    ` : endReached ? html`
                        <p>End reached.</p>
                    ` : null}
                `}
            `}
        </main>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

// @ts-ignore
customElements.define("user-followees-page", component(UserFolloweesPage, { useShadowDOM: false }))

/**
 * @param {ListFollowees} input 
 * @returns {Promise<Page<UserProfile>>}
 */
function fetchFollowees(input) {
    const u = new URL("/api/users/" + encodeURIComponent(input.username) + "/followees", window.location.origin)
    if (input.pageArgs?.first != null) {
        u.searchParams.set("first", input.pageArgs.first.toString())
    }
    if (input.pageArgs?.after != null) {
        u.searchParams.set("after", input.pageArgs.after)
    }
    if (input.pageArgs?.last != null) {
        u.searchParams.set("last", input.pageArgs.last.toString())
    }
    if (input.pageArgs?.before != null) {
        u.searchParams.set("before", input.pageArgs.before)
    }
    return request("GET", u.toString())
        .then(resp => resp.body)
}
