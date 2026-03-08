import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { repeat } from "lit/directives/repeat.js"
import { request } from "../http.js"
import "./intersectable-comp.js"
import "./toast-item.js"
import "./user-item.js"

/**
 * @typedef {import("../types.js").ListUserProfiles} ListUserProfiles
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

export default function () {
    return html`<search-page></search-page>`
}

function SearchPage() {
    const [users, setUsers] = useState([])
    const [usersEndCursor, setUsersEndCursor] = useState(null)
    const [fetching, setFetching] = useState(true)
    const [err, setErr] = useState(null)
    const [loadingMore, setLoadingMore] = useState(false)
    const [noMoreUsers, setNoMoreUsers] = useState(false)
    const [endReached, setEndReached] = useState(false)
    const [toast, setToast] = useState(null)

    const onNewResults = ev => {
        const { items: users, endCursor } = ev.detail
        setUsers(users)
        setUsersEndCursor(endCursor)
    }

    const loadMore = () => {
        if (loadingMore || noMoreUsers) {
            return
        }

        setLoadingMore(true)
        fetchUsers({search: getLocationSearchQuery(), pageArgs: {after: usersEndCursor}}).then(page => {
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
        fetchUsers({search: getLocationSearchQuery()}).then(page => {
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
    }, [])

    return html`
        <main class="container search-page">
            <h1>Search</h1>
            <search-form @new-results=${onNewResults}></search-form>
            ${err !== null ? html`
                <p class="error" role="alert">Could not fetch users: ${err.message}</p>
            ` : fetching ? html`
                <p class="loader" aria-busy="true" aria-live="polite">Loading users... please wait.</p>
            ` : html`
                ${users.length === 0 ? html`
                    <p>0 results</p>
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

customElements.define("search-page", component(SearchPage, { useShadowDOM: false }))

function SearchForm() {
    const [searchQuery, setSearchQuery] = useState(getLocationSearchQuery)
    const [fetching, setFetching] = useState(false)
    const [toast, setToast] = useState(null)

    const dispatchNewResults = payload => {
        this.dispatchEvent(new CustomEvent("new-results", { bubbles: true, detail: payload }))
    }

    const onSubmit = ev => {
        ev.preventDefault()
        history.pushState(history.state, document.title, "/search?q=" + encodeURIComponent(searchQuery))
        setFetching(true)
        fetchUsers({search: searchQuery}).then(dispatchNewResults, err => {
            const msg = "could not fetch users: " + err.message
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setFetching(false)
        })
    }

    const onInput = ev => {
        setSearchQuery(ev.currentTarget.value)
    }

    return html`
        <form @submit=${onSubmit}>
            <input type="search" name="q" placeholder="Search..." required .value=${searchQuery} .disabled=${fetching} @input=${onInput}>
        </form>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

customElements.define("search-form", component(SearchForm, { useShadowDOM: false }))

function getLocationSearchQuery() {
    try {
        const q = new URLSearchParams(location.search.substr(1))
        return q.has("q") ? decodeURIComponent(q.get("q")) : ""
    } catch (_) {
        return ""
    }
}

/**
 * @param {ListUserProfiles} input 
 * @returns {Promise<Page<UserProfile>>}
 */
function fetchUsers(input) {
    const u = new URL("/api/users", location.origin)
    if (input.search != null && input.search.trim() !== "") {
        u.searchParams.set("search", input.search)
    }
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
