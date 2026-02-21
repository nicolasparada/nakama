import { Textcomplete } from "@textcomplete/core"
import { TextareaEditor } from "@textcomplete/textarea"
import { component, useEffect, useRef, useState } from "haunted"
import { html } from "lit"
import { get as getTranslation, translate } from "lit-translate"
import { createRef, ref } from "lit/directives/ref.js"
import { repeat } from "lit/directives/repeat.js"
import { setLocalAuth } from "../auth.js"
import { authStore, useStore } from "../ctx.js"
import { request, subscribe } from "../http.js"
import "./intersectable-comp.js"
import "./post-item.js"
import "./toast-item.js"

const pageSize = 10

export default function () {
    return html`<home-page></home-page>`
}

function HomePage() {
    const [_, setAuth] = useStore(authStore)
    const [mode, setMode] = useState(() => {
        return localStorage.getItem("home-page-mode") === "timeline" ? "timeline" : "posts"
    })
    const [posts, setPosts] = useState(/** @type {import("../types.js").Post[]|import("../types.js").TimelineItem[]} */([]))
    const [endCursor, setEndCursor] = useState(/** @type {string|null} */(null))
    const [fetching, setFetching] = useState(posts.length === 0)
    const [err, setErr] = useState(/** @type {Error|null} */(null))
    const [loadingMore, setLoadingMore] = useState(false)
    const [noMore, setNoMore] = useState(false)
    const [endReached, setEndReached] = useState(false)
    const [queue, setQueue] = useState(/** @type {import("../types.js").Post[]|import("../types.js").TimelineItem[]} */([]))
    const [toast, setToast] = useState(/** @type {import("./toast-item.js").Toast|null} */(null))
    const postsRef = useRef(posts)
    const queueRef = useRef(queue)

    /**
     * @param {import("../types.js").Post[]|import("../types.js").TimelineItem[]} items 
     * @param {import("../types.js").TimelineItem} timelineItem 
     * @returns {boolean}
     */
    const hasTimelineItem = (items, timelineItem) => {
        return items.some(i =>
            i.id === timelineItem.id
            || (("timelineItemID" in i) && i.timelineItemID === timelineItem.timelineItemID)
        )
    }

    /**
     * @param {import("../types.js").Post[]} items 
     * @param {import("../types.js").Post} post 
     * @returns {boolean}
     */
    const hasPost = (items, post) => {
        return items.some(i => i.id === post.id)
    }

    const onTimelineItemCreated = ev => {
        const payload = /** @type {import("../types.js").TimelineItem} */ (ev.detail)
        setPosts(pp => [payload, ...queue, ...pp])
        setQueue([])
    }

    const onNewTimelineItemArrive = (/** @type {import("../types.js").TimelineItem} */ timelineItem) => {
        if (hasTimelineItem(postsRef.current, timelineItem) || hasTimelineItem(queueRef.current, timelineItem)) {
            return
        }

        setQueue(pp => [timelineItem, ...pp])
    }

    const onNewPostArrive = (/** @type {import("../types.js").Post} */ post) => {
        if (hasPost(postsRef.current, post) || hasPost(queueRef.current, post)) {
            return
        }
        
        setQueue(pp => [post, ...pp])
    }

    const onRemovedFromTimeline = ev => {
        const payload = /** @type {import("../types.js").TimelineItem} */ (ev.detail)
        setPosts(pp => pp.filter(p => !("timelineItemID" in p) || p.timelineItemID !== payload.timelineItemID))
    }

    const onPostDeleted = ev => {
        const payload = ev.detail
        setPosts(pp => pp.filter(p => p.id !== payload.id))
    }

    const onQueueBtnClick = () => {
        setPosts(pp => [...queue, ...pp])
        setQueue([])
    }

    const loadMore = () => {
        if (loadingMore || noMore) {
            return
        }

        setLoadingMore(true)
        const promise = mode === "timeline" ? fetchTimeline(endCursor) : fetchPosts(endCursor)
        promise.then(({ items: posts, endCursor }) => {
            setPosts(tt => [...tt, ...posts])
            setEndCursor(endCursor)

            if (posts.length < pageSize) {
                setNoMore(true)
                setEndReached(true)
            }
        }, err => {
            const msg = (mode === "timeline" ? "could not fetch more timeline items: " : "could not fetch more posts: ") + err.message
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setLoadingMore(false)
        })
    }

    const onTimelineModeClick = () => {
        setMode("timeline")
    }

    const onPostsModeClick = () => {
        setMode("posts")
    }

    useEffect(() => {
        localStorage.setItem("home-page-mode", mode)
    }, [mode])

    useEffect(() => {
        postsRef.current = posts
    }, [posts])

    useEffect(() => {
        queueRef.current = queue
    }, [queue])

    useEffect(() => {
        setPosts([])
        setEndCursor(null)
        setQueue([])
        setNoMore(false)
        setEndReached(false)

        setFetching(true)
        const promise = mode === "timeline" ? fetchTimeline() : fetchPosts()
        promise.then(({ items: posts, endCursor }) => {
            setPosts(posts)
            setEndCursor(endCursor)

            if (posts.length < pageSize) {
                setNoMore(true)
            }
        }, err => {
            console.error(mode === "timeline" ? "could not fetch timeline:" : "could not fetch posts:", err)
            if (err.name === "UnauthenticatedError") {
                setAuth(null)
                setLocalAuth(null)
            }

            setErr(err)
        }).finally(() => {
            setFetching(false)
        })
    }, [mode])

    useEffect(() => {
        return mode === "timeline" ?
            subscribeToTimeline(onNewTimelineItemArrive) :
            subscribeToPosts(onNewPostArrive)
    }, [mode])

    return html`
        <main class="container home-page">
            <h1>${mode === "timeline" ? translate("homePage.title.timeline") : translate("homePage.title.posts")}</h1>
            <post-form @timeline-item-created=${onTimelineItemCreated}></post-form>
            ${queue.length !== 0 ? html`
                <button class="queue-btn" @click=${onQueueBtnClick}>${mode === "timeline"
                ? (queue.length === 1
                    ? translate("homePage.queueBtn.newTimelineItem")
                    : translate("homePage.queueBtn.newTimelineItems", { length: queue.length }))
                : (queue.length === 1
                    ? translate("homePage.queueBtn.newPost")
                    : translate("homePage.queueBtn.newPosts", { length: queue.length }))}
                </button>
            ` : null}
            <div role="tablist">
                <button role="tab" id="${mode}-tab" aria-controls="${mode}-tabpanel" aria-selected=${mode === "posts" ? "true" : "false"} @click=${onPostsModeClick}>
                    ${translate("homePage.tabs.posts")}
                </button>
                <button role="tab" id="${mode}-tab" aria-controls="${mode}-tabpanel" aria-selected=${mode === "timeline" ? "true" : "false"} @click=${onTimelineModeClick}>
                    ${translate("homePage.tabs.timeline")}
                </button>
            </div>
            ${err !== null ? html`
                <p class="error" role="alert">${mode === "timeline"
                ? translate("homePage.err.timeline")
                : translate("homePage.err.posts")}
                    ${translate(err.name)}
                </p>
            ` : fetching ? html`
                <p class="loader" aria-busy="true" aria-live="polite">${mode === "timeline"
                ? translate("homePage.loading.timeline")
                : translate("homePage.loading.posts")}
                </p>
            ` : html`
                <div role="tabpanel" id="${mode}-tabpanel" aria-labelledby="${mode}-tab">
                ${posts.length === 0 ? html`
                    <p>${mode === "timeline"
                    ? translate("homePage.empty.timeline")
                    : translate("homePage.empty.posts")}
                    </p>
                ` : html`
                    <div class="posts" role="feed">
                        ${repeat(posts, p => p.id, p => html`<post-item .post=${p} .type=${mode === "timeline" ? "timeline_item" : "post"}
                            @removed-from-timeline=${onRemovedFromTimeline}
                            @resource-deleted=${onPostDeleted}></post-item>`)}
                    </div>
                    ${!noMore ? html`
                        <intersectable-comp @is-intersecting=${loadMore}></intersectable-comp>
                        <p class="loader" aria-busy="true" aria-live="polite">${mode === "timeline"
                        ? translate("homePage.loading.timeline")
                        : translate("homePage.loading.posts")}
                        </p>
                    ` : endReached ? html`
                        <p>${translate("homePage.end")}</p>
                    ` : null}
                `}
                </div>
            `}
        </main>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

customElements.define("home-page", component(HomePage, { useShadowDOM: false }))

const reMention = /\B@([\-+\w]*)$/

function PostForm() {
    const [auth] = useStore(authStore)
    const [content, setContent] = useState("")
    const [fetching, setFetching] = useState(false)
    const [nsfw, setNSFW] = useState(false)
    const [isSpoiler, setIsSpoiler] = useState(false)
    const [spoilerOf, setSpoilerOf] = useState("")
    const spoilerOfDialogRef = /** @type {import("lit/directives/ref.js").Ref<HTMLDialogElement>} */ (createRef())
    const [initialTextAreaHeight, setInitialTextAreaHeight] = useState(0)
    const mediaInputRef = /** @type {import("lit/directives/ref.js").Ref<HTMLInputElement>} */ (createRef())
    const textAreaRef = /** @type {import("lit/directives/ref.js").Ref<HTMLTextAreaElement>} */ (createRef())
    const textcompleteRef = useRef(/** @type {Textcomplete} */(null))
    const [toast, setToast] = useState(null)
    const [previews, setPreviews] = useState(/** @type {HTMLImageElement[]} */[])

    /**
     * @param {import("./../types.js").TimelineItem} timelineItem
     */
    const dispatchTimelineItemCreated = timelineItem => {
        this.dispatchEvent(new CustomEvent("timeline-item-created", { bubbles: true, detail: timelineItem }))
    }

    const onSubmit = ev => {
        ev.preventDefault()

        if (mediaInputRef.value === undefined) {
            return
        }

        const mediaInput = mediaInputRef.value

        let body

        if (mediaInput.files.length !== 0) {
            body = new FormData()
            body.set("content", content)
            if (spoilerOf.trim() !== "") {
                body.set("spoiler_of", spoilerOf.trim())
            }
            body.set("nsfw", JSON.stringify(nsfw))
            for (const file of mediaInput.files) {
                body.append("media", file)
            }
        } else {
            body = {
                content,
                spoilerOf: spoilerOf.trim() === "" ? null : spoilerOf.trim(),
                nsfw,
            }
        }

        setFetching(true)
        createTimelineItem(body).then(ti => {
            ti.user = auth.user
            setContent("")
            setNSFW(false)
            setIsSpoiler(false)
            setSpoilerOf("")
            setPreviews([])
            textcompleteRef.current.hide()
            mediaInput.value = ""

            dispatchTimelineItemCreated(ti)
        }, err => {
            const msg = getTranslation("postForm.err") + " " + getTranslation(err.name)
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setFetching(false)
        })
    }

    const onTextAreaInput = ev => {
        const el = /** @type {HTMLTextAreaElement} */ (ev.target)
        setContent(el.value)
    }

    const onNSFWInputChange = ev => {
        setNSFW(ev.currentTarget.checked)
    }

    const onIsSpoilerInputChange = ev => {
        const checked = ev.currentTarget.checked
        setIsSpoiler(checked)
        if (!checked) {
            setSpoilerOf("")
        } else {
            spoilerOfDialogRef.value.showModal()
        }
    }

    const onSpoilerOfInput = ev => {
        setSpoilerOf(ev.currentTarget.value)
    }

    const onSpoilerOfDialogClose = () => {
        if (spoilerOf.trim() === "") {
            setSpoilerOf("")
            setIsSpoiler(false)
        }
    }

    const onSpoilerOfFormSubmit = ev => {
        ev.preventDefault()
        spoilerOfDialogRef.value.close()
    }

    const onSpoilerOfCancelBtnClick = () => {
        setSpoilerOf("")
        setIsSpoiler(false)
        spoilerOfDialogRef.value.close()
    }

    const onMediaBtnClick = () => {
        if (mediaInputRef.value === null || fetching) {
            return
        }

        mediaInputRef.value.click()
    }

    const onMediaChange = ev => {
        if (mediaInputRef.value === null || mediaInputRef.value.files === null || mediaInputRef.value.files.length === 0) {
            return
        }

        /**
         * @type {HTMLImageElement[]}
         */
        const images = []
        for (const file of mediaInputRef.value.files) {
            const src = URL.createObjectURL(file)

            const img = document.createElement("img")
            img.addEventListener("load", () => {
                URL.revokeObjectURL(src)
            }, { once: true })
            img.src = src

            images.push(img)
        }

        setPreviews(images)
    }

    useEffect(() => {
        if (spoilerOfDialogRef.value === undefined) {
            return
        }

        const el = /** @type {HTMLDialogElement} */ (spoilerOfDialogRef.value)
        if ("HTMLDialogElement" in window && typeof el.showModal === "function") {
            return
        }

        import("dialog-polyfill").then(m => m.default).then(dialogPolyfill => {
            dialogPolyfill.registerDialog(el)
        }).catch(err => {
            console.error("could not import dialog polyfill:", err)
        })
    }, [spoilerOfDialogRef.value])

    useEffect(() => {
        if (textAreaRef.value === undefined) {
            return
        }

        const el = /** @type {HTMLTextAreaElement} */ (textAreaRef.value)

        const editor = new TextareaEditor(el)
        textcompleteRef.current = new Textcomplete(editor, [{
            match: reMention,
            search: async (term, cb) => {
                cb(await fetchUsernames(term).then(page => page.items, err => {
                    console.error("could not fetch mentions usernames:", err)
                    return []
                }))
            },
            replace: username => `@${username} `,
        }])

        setInitialTextAreaHeight(textAreaRef.value.scrollHeight)

        return () => {
            textcompleteRef.current.destroy()
        }
    }, [textAreaRef.value])

    // Share Target.
    useEffect(() => {
        const params = new URLSearchParams(window.location.search.slice(1))
        const preContent = []
        let cleanup = false
        if (params.has("text")) {
            cleanup = true
            const text = decodeURIComponent(params.get("text")).trim()
            if (text !== "") {
                preContent.push(text)
            }
        }
        if (params.has("url")) {
            cleanup = true
            const url = decodeURIComponent(params.get("url"))
            if (url !== "") {
                preContent.push(url)
            }
        }

        if (preContent.length !== 0) {
            setContent(preContent.join(" "))
            textAreaRef.value?.focus()
        }

        if (cleanup) {
            history.replaceState(history.state, document.title, "/")
        }
    }, [])

    useEffect(() => {
        if (textAreaRef.value === undefined) {
            return
        }

        const el = /** @type {HTMLTextAreaElement} */ (textAreaRef.value)

        el.style.height = initialTextAreaHeight + "px"
        if (el.value !== "") {
            el.style.height = Math.max(el.scrollHeight, initialTextAreaHeight) + "px"
        }
    }, [content])

    return html`
        <form class="post-form${content !== "" ? " has-content" : ""}" name="post-form" @submit=${onSubmit}>
            <textarea name="content"
                placeholder="${translate("postForm.placeholder")}"
                maxlenght="2048"
                aria-label="${translate("postForm.textAreaLabel")}"
                required
                .value=${content}
                .disabled=${fetching}
                ${ref(textAreaRef)}
                @input=${onTextAreaInput}></textarea>
            ${content !== "" ? html`
            <div class="post-form-controls">
                <div class="post-form-media">
                    <input type="file" name="media" accept="image/png,image/jpeg" multiple hidden @change=${onMediaChange} .disabled=${fetching} .ref=${ref(mediaInputRef)}>
                    <button type="button" .disabled=${fetching} @click=${onMediaBtnClick} title="Add media">
                        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="image"><rect width="24" height="24" opacity="0"/><path d="M18 3H6a3 3 0 0 0-3 3v12a3 3 0 0 0 3 3h12a3 3 0 0 0 3-3V6a3 3 0 0 0-3-3zM6 5h12a1 1 0 0 1 1 1v8.36l-3.2-2.73a2.77 2.77 0 0 0-3.52 0L5 17.7V6a1 1 0 0 1 1-1zm12 14H6.56l7-5.84a.78.78 0 0 1 .93 0L19 17v1a1 1 0 0 1-1 1z"/><circle cx="8" cy="8.5" r="1.5"/></g></g></svg>
                    </button>
                </div>
                <div class="post-form-options">
                    <label class="switch-wrapper">
                        <input type="checkbox" role="switch" name="nsfw" .disabled=${fetching} .checked=${nsfw} @change=${onNSFWInputChange}>
                        <span>${translate("postForm.nsfwLabel")}</span>
                    </label>
                    <label class="switch-wrapper">
                        <input type="checkbox" role="switch" name="is_spoiler" .disabled=${fetching} .checked=${isSpoiler} @change=${onIsSpoilerInputChange}>
                        <span>${spoilerOf.trim() === ""
                ? translate("postForm.spoilerLabel")
                : translate("postForm.spoilerOfLabel", { value: spoilerOf.trim() })}
                        </span>
                    </label>
                </div>
                <button class="submit-btn" .disabled=${fetching}>
                    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                        <g data-name="Layer 2">
                            <g data-name="paper-plane">
                                <rect width="24" height="24" opacity="0" />
                                <path
                                    d="M21 4a1.31 1.31 0 0 0-.06-.27v-.09a1 1 0 0 0-.2-.3 1 1 0 0 0-.29-.19h-.09a.86.86 0 0 0-.31-.15H20a1 1 0 0 0-.3 0l-18 6a1 1 0 0 0 0 1.9l8.53 2.84 2.84 8.53a1 1 0 0 0 1.9 0l6-18A1 1 0 0 0 21 4zm-4.7 2.29l-5.57 5.57L5.16 10zM14 18.84l-1.86-5.57 5.57-5.57z" />
                            </g>
                        </g>
                    </svg>
                    <span>${translate("postForm.submit")}</span>
                </button>
                </div>
            ` : null}
            ${previews.length !== 0 ? html`
                <ul class="media-scroller small" data-length="${previews.length}">
                ${previews.map(img => html`<li>${img}</li>`)}
                </ul>
            ` : null}
        </form>
        <dialog .ref=${ref(spoilerOfDialogRef)} @close=${onSpoilerOfDialogClose}>
            <form method="dialog" class="spoiler-of-form" @submit=${onSpoilerOfFormSubmit}>
                <label for="spoiler-of-input">${translate("postForm.dialog.spoilerOfLabel")}</label>
                <input type="text"
                    id="spoiler-of-input"
                    name="spoiler_of"
                    placeholder="${translate("postForm.dialog.spoilerOfPlaceholder")}"
                    maxlenght="64"
                    autocomplete="off"
                    .value=${spoilerOf}
                    ?required=${isSpoiler}
                    @input=${onSpoilerOfInput}>
                <div class="spoiler-of-controls">
                    <button>${translate("postForm.dialog.ok")}</button>
                    <button type="reset" @click=${onSpoilerOfCancelBtnClick}>${translate("postForm.dialog.cancel")}</button>
                </div>
            </form>
        </dialog>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

customElements.define("post-form", component(PostForm, { useShadowDOM: false }))

/**
 * @param {FormData|{content:string,spoilerOf?:string,nsfw?:boolean}} body
 */
function createTimelineItem(body) {
    return request("POST", "/api/timeline", { body })
        .then(resp => resp.body)
        .then(ti => {
            ti.createdAt = new Date(ti.createdAt)
            return ti
        })
}

function subscribeToTimeline(cb) {
    return subscribe("/api/timeline", ti => {
        ti.createdAt = new Date(ti.createdAt)
        cb(ti)
    })
}

function fetchTimeline(before = "", last = pageSize) {
    return request("GET", `/api/timeline?last=${encodeURIComponent(last)}&before=${encodeURIComponent(before)}`)
        .then(resp => resp.body)
        .then(page => {
            page.items = page.items.map(ti => ({
                ...ti,
                createdAt: new Date(ti.createdAt),
            }))
            return page
        })
}

function subscribeToPosts(cb) {
    return subscribe("/api/posts", p => {
        p.createdAt = new Date(p.createdAt)
        cb(p)
    })
}

function fetchPosts(before = "", last = pageSize) {
    return request("GET", `/api/posts?last=${encodeURIComponent(last)}&before=${encodeURIComponent(before)}`)
        .then(resp => resp.body)
        .then(page => {
            page.items = page.items.map(p => ({
                ...p,
                createdAt: new Date(p.createdAt),
            }))
            return page
        })
}

function fetchUsernames(startingWith = "", after = "", first = pageSize) {
    return request("GET", `/api/usernames?starting_with=${encodeURIComponent(startingWith)}&after=${encodeURIComponent(after)}&first=${encodeURIComponent(first)}`)
        .then(resp => resp.body)
}
