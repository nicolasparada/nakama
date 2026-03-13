import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { repeat } from "lit/directives/repeat.js"
import { setLocalAuth } from "../auth.js"
import { authStore, useStore } from "../ctx.js"
import { request } from "../http.js"
import "./intersectable-comp.js"
import "./post-item.js"
import "./toast-item.js"

/**
 * @typedef {import("../types.js").Post} Post
 */

/**
 * @typedef {import("../types.js").ListPosts} ListPosts
 */

/**
 * @template T
 * @typedef {import("../types.js").Page<T>} Page
 */

/**
 * @typedef {import("./toast-item.js").Toast} Toast
 */

/**
 * @param {object} props
 * @param {object} props.params
 * @param {string} props.params.tag
 */
export default function ({ params }) {
    return html`<tagged-posts-page .tag=${params.tag}></tagged-posts-page>`
}

/**
 * @param {object} props
 * @param {string} props.tag
 */
function TaggedPostsPage({ tag }) {
    const [_, setAuth] = useStore(authStore)
    const [posts, setPosts] = useState([])
    const [endCursor, setEndCursor] = useState(null)
    const [fetching, setFetching] = useState(posts.length === 0)
    const [err, setErr] = useState(null)
    const [loadingMore, setLoadingMore] = useState(false)
    const [noMore, setNoMore] = useState(false)
    const [endReached, setEndReached] = useState(false)
    const [toast, setToast] = useState(null)

    const onPostDeleted = ev => {
        const payload = ev.detail
        setPosts(pp => pp.filter(p => p.id !== payload.id))
    }

    const loadMore = () => {
        if (loadingMore || noMore) {
            return
        }

        setLoadingMore(true)
        fetchPosts({ tag, pageArgs: { after: endCursor } }).then(page => {
            setPosts(tt => [...tt, ...page.items])
            setEndCursor(page.pageInfo.endCursor)

            if (!page.pageInfo.hasNextPage) {
                setNoMore(true)
                setEndReached(true)
            }
        }, err => {
            const msg = "could not fetch more tagged posts: " + err.message
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setLoadingMore(false)
        })
    }

    useEffect(() => {
        setPosts([])
        setEndCursor(null)
        setNoMore(false)
        setEndReached(false)

        setFetching(true)
        fetchPosts({tag}).then(page => {
            setPosts(page.items)
            setEndCursor(page.pageInfo.endCursor)

            if (!page.pageInfo.hasNextPage) {
                setNoMore(true)
            }
        }, err => {
            console.error("could not fetch tagged posts:", err)
            if (err.name === "UnauthenticatedError") {
                setAuth(null)
                setLocalAuth(null)
            }

            setErr(err)
        }).finally(() => {
            setFetching(false)
        })
    }, [tag])

    return html`
        <main class="container tagged-posts-page">
            <h1>"${tag}" Tagged Posts</h1>
            ${err !== null ? html`
                <p class="error" role="alert">
                    could not fetch tagged posts: ${err.message}
                </p>
            ` : fetching ? html`
                <p class="loader" aria-busy="true" aria-live="polite">
                    Loading tagged posts... please wait.
                </p>
            ` : html`
                <div role="tabpanel" id="tabpanel" aria-labelledby="tab">
                ${posts.length === 0 ? html`
                    <p>0 posts</p>
                ` : html`
                    <div class="posts" role="feed">
                        ${repeat(posts, p => p.id, p => html`<post-item .post=${p} .type=${"post"}
                            @resource-deleted=${onPostDeleted}></post-item>`)}
                    </div>
                    ${!noMore ? html`
                        <intersectable-comp @is-intersecting=${loadMore}></intersectable-comp>
                        <p class="loader" aria-busy="true" aria-live="polite">
                            Loading tagged posts... please wait.
                        </p>
                    ` : endReached ? html`
                        <p>End reached</p>
                    ` : null}
                `}
                </div>
            `}
        </main>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

// @ts-ignore
customElements.define("tagged-posts-page", component(TaggedPostsPage, { useShadowDOM: false }))

/**
 * @param {ListPosts} input
 * @returns {Promise<Page<Post>>}
 */
function fetchPosts(input) {
    const u = new URL("/api/posts", window.location.origin)
    if (input.tag != null) {
        u.searchParams.set("tag", input.tag)
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
        .then((/** @type {Page<Post>} */ page) => {
            page.items = page.items.map(p => ({
                ...p,
                createdAt: new Date(p.createdAt),
            }))
            return page
        })
}