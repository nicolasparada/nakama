import fetchJSONP from "fetch-jsonp"
import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { get as getTranslation, translate } from "lit-translate"
import { ifDefined } from "lit/directives/if-defined.js"
import { createRef, ref } from "lit/directives/ref.js"
import { unsafeHTML } from "lit/directives/unsafe-html.js"
import mediumZoom from "medium-zoom"
import { authStore, useStore } from "../ctx.js"
import { request } from "../http.js"
import { collectMediaURLs, linkify } from "../utils.js"
import { Avatar } from "./avatar.js"
import "./relative-datetime.js"
import "./toast-item.js"

/**
 * @param {object} props
 * @param {import("../types.js").Post|import("../types.js").Comment|import("../types.js").TimelineItem} props.post
 * @param {"timeline_item"|"post"|"comment"} props.type
 */
function PostItem({ post: initialPost, type }) {
    const [auth] = useStore(authStore)
    const [post, setPost] = useState(initialPost)
    const [mediaURLs, setMediaURLs] = useState([])
    const [showMenu, setShowMenu] = useState(false)
    const [togglingPostSubscription, setTogglingPostSubscription] = useState(false)
    const [updating, setUpdating] = useState(false)
    const [removingFromTimeline, setRemovingFromTimeline] = useState(false)
    const [deleting, setDeleting] = useState(false)
    const [displaySpoiler, setDisplaySpoiler] = useState(false)
    const [displayNSFW, setDisplayNSFW] = useState(false)
    const [toast, setToast] = useState(null)
    const [postCanBeUpdated, setPostCanBeUpdated] = useState(canUpdatePost(post))

    const onMenuBtnClick = () => {
        setShowMenu(v => !v)
    }

    const onMenuWrapperBlur = ev => {
        if (ev.relatedTarget === null || !ev.currentTarget.closest(".post-menu-wrapper").contains(ev.relatedTarget)) {
            setShowMenu(false)
        }
    }

    const onPostSubscriptionToggleBtnClick = () => {
        setTogglingPostSubscription(true)
        togglePostSubscription(post.id).then(payload => {
            setPost(p => ({
                ...p,
                ...payload,
            }))
        }, err => {
            const msg = getTranslation("postItem.errToggleSubscription") + " " + getTranslation(err.name)
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setTogglingPostSubscription(false)
        })
    }

    const onUpdateBtnClick = () => {
        setUpdating(true)

        const content = prompt(getTranslation("postItem.menu.edit"), post.content)
        if (content === "" || content === null || content === post.content) {
            setUpdating(false)
            return
        }

        const fn = type === "comment" ? updateComment : updatePost

        fn(post.id, { content }).then(updated => {
            setPost(p => ({
                ...p,
                ...updated,
            }))
        }, err => {
            const msg = getTranslation("postItem.errUpdate") + " " + getTranslation(err.name)
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setUpdating(false)
        })
    }

    const dispatchRemovedFromTimeline = payload => {
        this.dispatchEvent(new CustomEvent("removed-from-timeline", { bubbles: true, detail: payload }))
    }

    const onRemoveFromTimelineBtnClick = () => {
        const ti = /** @type {import("../types.js").TimelineItem} */ (post)
        if (type !== "timeline_item") {
            return
        }

        setRemovingFromTimeline(true)
        removeTimelineItem(ti.timelineItemID).then(() => {
            dispatchRemovedFromTimeline({ timelineItemID: ti.timelineItemID })
        }, err => {
            const msg = getTranslation("postItem.errRemove") + " " + getTranslation(err.name)
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setRemovingFromTimeline(false)
        })
    }

    const dispatchResourceDeleted = payload => {
        this.dispatchEvent(new CustomEvent("resource-deleted", { bubbles: true, detail: payload }))
    }

    const onDeleteBtnClick = () => {
        setDeleting(true)
        deleteResource(type, post.id).then(() => {
            dispatchResourceDeleted({ id: post.id })
        }, err => {
            const msg = getTranslation("postItem.errDelete.fmt", { type: getTranslation("postItem.errDelete.types." + type) })
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setDeleting(false)
        })
    }

    const onDisplaySpoilerBtnClick = () => {
        setDisplaySpoiler(true)
    }

    const onDisplayNSFWBtnClick = () => {
        setDisplayNSFW(true)
    }

    const onNewReactionCounts = ev => {
        const payload = ev.detail
        setPost(p => ({
            ...p,
            ...payload,
        }))
    }

    useEffect(() => {
        const id = setInterval(() => {
            setPostCanBeUpdated(canUpdatePost(post))
        }, 1000 * 30) // 30 seconds
        return () => {
            clearInterval(id)
        }
    }, [post])

    useEffect(() => {
        const urls = []
        if ("mediaURLs" in post) {
            for (const mediaURL of post.mediaURLs) {
                urls.push(new URL(mediaURL, location.origin))
            }
        }
        urls.push(...collectMediaURLs(post.content))
        setMediaURLs(urls)
    }, [post["mediaURLs"], post.content])

    useEffect(() => {
        setPost(initialPost)
    }, [initialPost])

    return html`
        <article class="post">
            <div class="post-header">
                <a href="/@${post.user.username}" class="post-author">
                    ${Avatar(post.user)}
                    <span class="username">${post.user.username}</span>
                </a>
                <div class="post-meta">
                    ${type === "comment" ? html`
                        <relative-datetime class="post-ts" .datetime=${post.createdAt}></relative-datetime>
                    ` : html`
                        <a href="/posts/${post.id}" class="post-ts">
                            <relative-datetime .datetime=${post.createdAt}></relative-datetime>
                        </a>
                    `}
                    ${auth !== null && !(type === "comment" && !post.mine) ? html`
                        <div class="post-menu-wrapper">
                            <button class="post-menu-wrapper-btn"
                                id="${post.id}-more-menu-btn"
                                aria-haspopup="true"
                                aria-controls="${post.id}-more-menu"
                                title="${translate("postItem.menu.title")}"
                                aria-expanded="${ifDefined(showMenu ? "true" : undefined)}"
                                @click=${onMenuBtnClick}
                                @blur=${onMenuWrapperBlur}>
                                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="more-vertical"><rect width="24" height="24" transform="rotate(-90 12 12)" opacity="0"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="5" r="2"/><circle cx="12" cy="19" r="2"/></g></g></svg>
                            </button>
                            <ul class="post-menu"
                                id="${post.id}-more-menu"
                                role="menu"
                                aria-labelledby="${post.id}-more-menu-btn"
                                tabindex="-1"
                                @blur=${onMenuWrapperBlur}>
                                ${type === "timeline_item" || type === "post" ? html`
                                    <li class="post-menu-item" role="none">
                                        <button class="post-menu-btn"
                                            role="menuitem"
                                            tabindex="-1"
                                            .disabled=${togglingPostSubscription}
                                            @click=${onPostSubscriptionToggleBtnClick}
                                            @blur=${onMenuWrapperBlur}>
                                            ${"subscribed" in post && post.subscribed ? html`
                                                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="bell-off"><rect width="24" height="24" opacity="0"/><path d="M8.9 5.17A4.67 4.67 0 0 1 12.64 4a4.86 4.86 0 0 1 4.08 4.9v4.5a1.92 1.92 0 0 0 .1.59l3.6 3.6a1.58 1.58 0 0 0 .45-.6 1.62 1.62 0 0 0-.35-1.78l-1.8-1.81V8.94a6.86 6.86 0 0 0-5.82-6.88 6.71 6.71 0 0 0-5.32 1.61 6.88 6.88 0 0 0-.58.54l1.47 1.43a4.79 4.79 0 0 1 .43-.47z"/><path d="M14 16.86l-.83-.86H5.51l1.18-1.18a2 2 0 0 0 .59-1.42v-3.29l-2-2a5.68 5.68 0 0 0 0 .59v4.7l-1.8 1.81A1.63 1.63 0 0 0 4.64 18H8v.34A3.84 3.84 0 0 0 12 22a3.88 3.88 0 0 0 4-3.22l-.83-.78zM12 20a1.88 1.88 0 0 1-2-1.66V18h4v.34A1.88 1.88 0 0 1 12 20z"/><path d="M20.71 19.29L19.41 18l-2-2-9.52-9.53L6.42 5 4.71 3.29a1 1 0 0 0-1.42 1.42L5.53 7l1.75 1.7 7.31 7.3.07.07L16 17.41l.59.59 2.7 2.71a1 1 0 0 0 1.42 0 1 1 0 0 0 0-1.42z"/></g></g></svg>
                                                <span>${translate("postItem.menu.unsusbcribe")}</span>
                                            ` : html`
                                                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="bell"><rect width="24" height="24" opacity="0"/><path d="M20.52 15.21l-1.8-1.81V8.94a6.86 6.86 0 0 0-5.82-6.88 6.74 6.74 0 0 0-7.62 6.67v4.67l-1.8 1.81A1.64 1.64 0 0 0 4.64 18H8v.34A3.84 3.84 0 0 0 12 22a3.84 3.84 0 0 0 4-3.66V18h3.36a1.64 1.64 0 0 0 1.16-2.79zM14 18.34A1.88 1.88 0 0 1 12 20a1.88 1.88 0 0 1-2-1.66V18h4zM5.51 16l1.18-1.18a2 2 0 0 0 .59-1.42V8.73A4.73 4.73 0 0 1 8.9 5.17 4.67 4.67 0 0 1 12.64 4a4.86 4.86 0 0 1 4.08 4.9v4.5a2 2 0 0 0 .58 1.42L18.49 16z"/></g></g></svg>
                                                <span>${translate("postItem.menu.susbcribe")}</span>
                                            `}
                                        </button>
                                    </li>
                                ` : null}
                                ${post.mine && postCanBeUpdated ? html`
                                    <li class="post-menu-item" role="none">
                                        <button class="post-menu-btn" role="menuitem" tabindex="-1" .disabled=${updating} @click=${onUpdateBtnClick} @blur=${onMenuWrapperBlur}>
                                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="edit"><rect width="24" height="24" opacity="0"/><path d="M19.4 7.34L16.66 4.6A2 2 0 0 0 14 4.53l-9 9a2 2 0 0 0-.57 1.21L4 18.91a1 1 0 0 0 .29.8A1 1 0 0 0 5 20h.09l4.17-.38a2 2 0 0 0 1.21-.57l9-9a1.92 1.92 0 0 0-.07-2.71zM9.08 17.62l-3 .28.27-3L12 9.32l2.7 2.7zM16 10.68L13.32 8l1.95-2L18 8.73z"/></g></g></svg>
                                            <span>${translate("postItem.menu.edit")}</span>
                                        </button>
                                    </li>
                                ` : null}
                                ${type === "timeline_item" ? html`
                                    <li class="post-menu-item" role="none">
                                        <button class="post-menu-btn" role="menuitem" tabindex="-1" .disabled=${removingFromTimeline} @click=${onRemoveFromTimelineBtnClick} @blur=${onMenuWrapperBlur}>
                                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="eye-off"><rect width="24" height="24" opacity="0"/><path d="M4.71 3.29a1 1 0 0 0-1.42 1.42l5.63 5.63a3.5 3.5 0 0 0 4.74 4.74l5.63 5.63a1 1 0 0 0 1.42 0 1 1 0 0 0 0-1.42zM12 13.5a1.5 1.5 0 0 1-1.5-1.5v-.07l1.56 1.56z"/><path d="M12.22 17c-4.3.1-7.12-3.59-8-5a13.7 13.7 0 0 1 2.24-2.72L5 7.87a15.89 15.89 0 0 0-2.87 3.63 1 1 0 0 0 0 1c.63 1.09 4 6.5 9.89 6.5h.25a9.48 9.48 0 0 0 3.23-.67l-1.58-1.58a7.74 7.74 0 0 1-1.7.25z"/><path d="M21.87 11.5c-.64-1.11-4.17-6.68-10.14-6.5a9.48 9.48 0 0 0-3.23.67l1.58 1.58a7.74 7.74 0 0 1 1.7-.25c4.29-.11 7.11 3.59 8 5a13.7 13.7 0 0 1-2.29 2.72L19 16.13a15.89 15.89 0 0 0 2.91-3.63 1 1 0 0 0-.04-1z"/></g></g></svg>
                                            <span>${translate("postItem.menu.remove")}</span>
                                        </button>
                                    </li>
                                ` : null}
                                ${post.mine ? html`
                                    <li class="post-menu-item" role="none">
                                        <button class="post-menu-btn" role="menuitem" tabindex="-1" .disabled=${deleting} @click=${onDeleteBtnClick} @blur=${onMenuWrapperBlur}>
                                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="trash-2"><rect width="24" height="24" opacity="0"/><path d="M21 6h-5V4.33A2.42 2.42 0 0 0 13.5 2h-3A2.42 2.42 0 0 0 8 4.33V6H3a1 1 0 0 0 0 2h1v11a3 3 0 0 0 3 3h10a3 3 0 0 0 3-3V8h1a1 1 0 0 0 0-2zM10 4.33c0-.16.21-.33.5-.33h3c.29 0 .5.17.5.33V6h-4zM18 19a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V8h12z"/><path d="M9 17a1 1 0 0 0 1-1v-4a1 1 0 0 0-2 0v4a1 1 0 0 0 1 1z"/><path d="M15 17a1 1 0 0 0 1-1v-4a1 1 0 0 0-2 0v4a1 1 0 0 0 1 1z"/></g></g></svg>
                                            <span>${translate("postItem.menu.delete")}</span>
                                        </button>
                                    </li>
                                ` : null}
                            </ul>
                        </div>
                    ` : null}
                </div>
            </div>
            <div class="post-content">
                ${"spoilerOf" in post && post.spoilerOf !== null && !displaySpoiler ? html`
                    <div class="post-warning">
                        <p>${translate("postItem.spoiler.warning")} ${post.spoilerOf}</p>
                        <button @click=${onDisplaySpoilerBtnClick}>${translate("postItem.spoiler.show")}</button>
                    </div>
                ` : html`
                    <p>${unsafeHTML(linkify(post.content))}</p>
                    ${"nsfw" in post && post.nsfw && !displayNSFW ? html`
                        <div class="post-warning">
                            <p>${translate("postItem.nsfw.warning")}</p>
                            <button @click=${onDisplayNSFWBtnClick}>${translate("postItem.nsfw.show")}</button>
                        </div>
                    ` : html`
                        <media-scroller .urls=${mediaURLs}></media-scroller>
                    `}
                `}
            </div>
            <div class="post-footer">
                ${post.reactions.length !== 0 || auth !== null ? html`
                    <div class="post-reactions">
                        ${post.reactions.length !== 0 ? post.reactions.map(r => html`
                            <reaction-btn .postID=${post.id} .reaction=${r} .type=${type} @new-reaction-counts=${onNewReactionCounts}></reaction-btn>
                        `) : null}
                        ${auth !== null ? html`
                            <add-reaction-btn .postID=${post.id} .type=${type} @new-reaction-counts=${onNewReactionCounts}></add-reaction-btn>
                        ` : null}
                    </div>
                ` : null}
                ${"commentsCount" in post ? html`
                    <a class="post-replies-link btn" href="/posts/${post.id}" title="${translate("postItem.comments")}">
                        <span>${post.commentsCount}</span>
                        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="message-square"><rect width="24" height="24" opacity="0"/><circle cx="12" cy="11" r="1"/><circle cx="16" cy="11" r="1"/><circle cx="8" cy="11" r="1"/><path d="M19 3H5a3 3 0 0 0-3 3v15a1 1 0 0 0 .51.87A1 1 0 0 0 3 22a1 1 0 0 0 .51-.14L8 19.14a1 1 0 0 1 .55-.14H19a3 3 0 0 0 3-3V6a3 3 0 0 0-3-3zm1 13a1 1 0 0 1-1 1H8.55a3 3 0 0 0-1.55.43l-3 1.8V6a1 1 0 0 1 1-1h14a1 1 0 0 1 1 1z"/></g></g></svg>
                    </a>
                ` : null}
            </div>
        </article>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

// @ts-ignore
customElements.define("post-item", component(PostItem, { useShadowDOM: false }))

function ReactionBtn({ postID, reaction: initialReaction, type }) {
    const [reaction, setReaction] = useState(initialReaction)
    const [fetching, setFetching] = useState(false)
    const [toast, setToast] = useState(null)

    const dispatchNewReactionCounts = payload => {
        this.dispatchEvent(new CustomEvent("new-reaction-counts", { bubbles: true, detail: payload }))
    }

    const onClick = () => {
        setFetching(true)
        toggleReaction(type, postID, reaction).then(reactions => {
            dispatchNewReactionCounts({ reactions })
        }, err => {
            const msg = getTranslation("reactionBtn.err") + " " + getTranslation(err.name)
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setFetching(false)
        })
    }

    useEffect(() => {
        setReaction(initialReaction)
    }, [initialReaction])

    return html`
        <button class="post-reaction${reaction.reacted ? " reacted" : ""}" .disabled=${fetching} @click=${onClick}>
            <span>${reaction.count}</span>
            ${reaction.kind === "emoji" ? html`
                <span>${reaction.reaction}</span>
            ` : html`
                <img src="${reaction.reaction}">
            `}
        </button>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

// @ts-ignore
customElements.define("reaction-btn", component(ReactionBtn, { useShadowDOM: false }))

const emojiPickerStyles = `
    .picker {
        border-radius: var(--emoji-picker-border-radius);
        border-top-left-radius: var(--emoji-picker-border-top-left-radius);
        border-bottom-left-radius: var(--emoji-picker-border-bottom-left-radius);
        border-bottom-right-radius: var(--emoji-picker-border-bottom-right-radius);
        border: var(--emoji-picker-border);
    }

    .picker input.search {
        background-color: var(--emoji-picker-input-background-color);
        border: var(--emoji-picker-input-border);
        height: var(--emoji-picker-input-height);
        padding: var(--emoji-picker-input-padding);
    }
`

function AddReactionBtn({ postID, type }) {
    const emojiPickerRef = /** @type {import("lit/directives/ref.js").Ref<import("emoji-picker-element").Picker>} */(createRef())
    const [showEmojiPicker, setShowEmojiPicker] = useState(false)
    const [fetching, setFetching] = useState(false)
    const [toast, setToast] = useState(null)

    const dispatchNewReactionCounts = payload => {
        this.dispatchEvent(new CustomEvent("new-reaction-counts", { bubbles: true, detail: payload }))
    }

    const onAddReactionBtnClick = () => {
        setShowEmojiPicker(hidden => !hidden)
    }

    const onEmojiPickerWrapperBlur = ev => {
        if (ev.relatedTarget === null || !ev.currentTarget.closest(".emoji-picker-wrapper").contains(ev.relatedTarget)) {
            setShowEmojiPicker(false)
        }
    }

    const onEmojiClick = ev => {
        const emoji = ev.detail.unicode
        setFetching(true)
        toggleReaction(type, postID, { kind: "emoji", reaction: emoji }).then(reactions => {
            setShowEmojiPicker(false)
            dispatchNewReactionCounts({ reactions })
        }, err => {
            const msg = getTranslation("addReactionBtn.err") + " " + getTranslation(err.name)
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setFetching(false)
        })
    }

    useEffect(() => {
        if (emojiPickerRef.value === undefined) {
            return
        }

        const el = /** @type {import("emoji-picker-element").Picker} */(emojiPickerRef.value)

        const styleEmojiPicker = () => {
            try {
                if (el !== undefined && el.shadowRoot !== null) {
                    const style = document.createElement("style")
                    style.textContent = emojiPickerStyles
                    el.shadowRoot.appendChild(style)
                }
            } catch (_) { }
        }

        if (customElements.get("emoji-picker") === undefined) {
            import("emoji-picker-element").then(styleEmojiPicker).catch(() => { })
        } else {
            styleEmojiPicker()
        }
    }, [emojiPickerRef.value])

    return html`
        <div class="emoji-picker-wrapper">
            <button class="post-add-reaction-btn"
                id="${postID}-reactions-menu-btn"
                aria-haspopup="true"
                aria-controls="${postID}-reactions-menu"
                aria-expanded="${ifDefined(showEmojiPicker ? "true" : undefined)}"
                title="${translate("addReactionBtn.title")}"
                .disabled=${fetching}
                @click=${onAddReactionBtnClick}
                @blur=${onEmojiPickerWrapperBlur}>
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><defs><style></style></defs><g id="Layer_2" data-name="Layer 2"><g id="smiling-face"><g id="smiling-face" data-name="smiling-face"><rect width="24" height="24" opacity="0"/><path d="M12 2c5.523 0 10 4.477 10 10s-4.477 10-10 10S2 17.523 2 12 6.477 2 12 2zm0 2a8 8 0 1 0 0 16 8 8 0 0 0 0-16zm5 9a5 5 0 0 1-10 0z" id="🎨-Icon-Сolor"/></g></g></g></svg>
            </button>
            <emoji-picker ${ref(emojiPickerRef)}
                class="dark${fetching ? " disabled" : ""}"
                id="${postID}-reactions-menu"
                role="menu"
                aria-labelledby="${postID}-more-menu-btn"
                tabindex="-1"
                @emoji-click=${onEmojiClick}
                @blur=${onEmojiPickerWrapperBlur}></emoji-picker>
        </div>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

// @ts-ignore
customElements.define("add-reaction-btn", component(AddReactionBtn, { useShadowDOM: false }))

const trustedOrigins = ["https://i.imgur.com", "https://puu.sh", location.origin]
const imageExts = ["jpg", "jpeg", "gif", "png", "webp", "avif"].map(ext => "." + ext)
const audioExts = ["wav", "mp3", "flac"].map(ext => "." + ext)
const videoExts = ["mp4", "webm", "mov", "3gp", "ogg"].map(ext => "." + ext)

/**
 *
 * @param {{urls:URL[]}} props
 * @returns
 */
function MediaScroller({ urls }) {
    const [items, setItems] = useState([])

    useEffect(() => {
        void async function collectItems() {
            const items = []
            for (const url of urls) {
                {
                    const result = findYouTubeID(url)
                    if (result.id !== null) {
                        items.push(html`<iframe
                            src="https://www.youtube-nocookie.com/embed/${result.id}${result.seconds !== null ? "?start=" + result.seconds : ""}"
                            title="YouTube video player"
                            frameborder="0"
                            allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                            allowfullscreen></iframe>`)
                        continue
                    }
                }

                {
                    const vimeoID = findVimeoID(url)
                    if (vimeoID !== null) {
                        items.push(html`<iframe
                            src="https://player.vimeo.com/video/${vimeoID}?byline=0&portrait=0"
                            title="Vimeo video player"
                            frameborder="0"
                            allow="autoplay; fullscreen; picture-in-picture"
                            allowfullscreen></iframe>`)
                        continue
                    }
                }

                {
                    const tweetID = findTweetID(url)
                    if (tweetID !== null) {
                        try {
                            const u = "https://publish.twitter.com/oembed?dnt=true&hide_thread=true&omit_script=1&theme=dark&border_color=23a80000&align=center&chrome=" + encodeURIComponent("transparent noborders") + "&url=" + encodeURIComponent("https://twitter.com" + url.pathname)
                            const resp = await fetchJSONP(u)
                            const json = await resp.json()
                            if (typeof json === "object" && json !== null && typeof json.html === "string") {
                                const div = document.createElement("div")
                                div.innerHTML = json.html
                                items.push(html`${div}`)
                                setTimeout(() => {
                                    addTwitterWidget().then(() => {
                                        if ("twttr" in window) {
                                            // @ts-ignore
                                            window.twttr.widgets.load(div)
                                        }
                                    })
                                }, 1)
                                continue
                            }
                        } catch (err) {
                            console.error("failed to load tweet", err)
                        }
                    }
                }

                if (url.hostname === "bsky.app" && url.pathname.includes("/post/")) {
                    try {
                        const endpoint = "https://embed.bsky.app/oembed?url=" + encodeURIComponent(url.toString())
                        const resp = await fetch("/api/proxy?target=" + encodeURIComponent(endpoint))
                        if (!resp.ok) {
                            continue
                        }

                        const json = await resp.json()
                        if (typeof json === "object" && json !== null && typeof json.html === "string") {
                            const div = document.createElement("div")
                            div.innerHTML = json.html
                            const script = div.querySelector("script")
                            if (script !== null) {
                                script.remove()
                            }
                            items.push(html`${div}`)
                            setTimeout(() => {
                                addBskyWidget().then(() => {
                                    if ("bluesky" in window) {
                                        // @ts-ignore
                                        window.bluesky.scan(div)
                                    }
                                })
                            }, 1)
                            continue
                        }
                    } catch (err) {
                        console.error("failed to load bsky.app post", err)
                    }
                }

                {
                    const id = findCoubVideoID(url)
                    if (id !== null) {
                        items.push(html`<iframe
                            src="https://coub.com/embed/${id}?muted=false&autostart=false&originalSize=true&startWithHD=true"
                            title="Coub video player"
                            frameborder="0"
                            allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                            allowfullscreen></iframe>`)
                        continue
                    }
                }

                {
                    if (trustedOrigins.includes(url.origin) || (trustedOrigins.some(o => o.includes("localhost") && url.hostname === "localhost"))) {
                        if (imageExts.some(ext => url.pathname.endsWith(ext))) {
                            items.push(html`<zoomable-img .src=${url.toString()}></zoomable-img>`)
                            continue
                        }

                        if (audioExts.some(ext => url.pathname.endsWith(ext))) {
                            items.push(html`<audio src="${url.toString()}" preload="metadata" controls loop></audio>`)
                            continue
                        }

                        if (videoExts.some(ext => url.pathname.endsWith(ext))) {
                            items.push(html`<video src="${url.toString()}" preload="metadata" controls loop></video>`)
                            continue
                        }
                    }
                }

                {
                    const result = findTikTokVideoID(url)
                    if (result !== null) {
                        try {
                            const resp = await fetch("https://www.tiktok.com/oembed?url=" + encodeURIComponent(url.toString()))
                            if (!resp.ok) {
                                continue
                            }

                            const json = await resp.json()
                            items.push(html`
                                <a href=${url.toString()} target="_blank" rel="noopener noreferrer">
                                    <img src=${json.thumbnail_url} width=${json.thumbnail_width} height=${json.thumbnail_height} loading="lazy"></img>
                                </a>
                            `)
                        } catch (_) { }
                        continue
                    }
                }

                try {
                    const endpoint = "/api/proxy?target=" + encodeURIComponent(url.toString())
                    const resp = await fetch(endpoint, {
                        method: "HEAD",
                        headers: {
                            accept: "image/*, audio/*, video/*",
                        },
                    })
                    if (!resp.ok) {
                        continue
                    }

                    const ct = resp.headers.get("content-type")
                    if (ct === null) {
                        continue
                    }

                    const parts = ct.split("/")
                    if (parts.length === 0) {
                        continue
                    }

                    switch (parts[0]) {
                        case "image":
                            items.push(html`<zoomable-img .src=${endpoint}></zoomable-img>`)
                            break
                        case "audio":
                            items.push(html`<audio src="${endpoint}" preload="metadata" controls loop></audio>`)
                            break
                        case "video":
                            items.push(html`<video src="${endpoint}" preload="metadata" controls loop></video>`)
                            break
                    }
                } catch (_) { }
            }
            setItems(items)
        }()
    }, [urls])

    if (items.length === 0) {
        return null
    }

    return html`
        <ul class="media-scroller" data-length="${items.length}">
            ${items.map(item => html`
                <li>${item}</li>
            `)}
        </ul>
    `
}


/**
 * addScript makes sure to insert the script to the document body once.
 * It resolves only when the symbol is present in the window object.
 * When the script loads, it checks if the symbol is present in the window object as well.
 * When the script fails to load or the symbol is not present in the window object after a timeout, it rejects.
 * @param {string} src
 * @param {string} symbol
 */
function addScript(src, symbol) {
    if (symbol in window) {
        return Promise.resolve()
    }

    return new Promise((resolve, reject) => {
        const script = document.createElement("script")
        script.src = src
        script.async = true
        script.onload = () => {
            if (symbol in window) {
                resolve()
                return
            }

            let tries = 0

            const interval = setInterval(() => {
                if (symbol in window) {
                    clearInterval(interval)
                    resolve()
                    return
                }

                if (tries++ > 10) {
                    clearInterval(interval)
                    reject(new Error("symbol '" + symbol + "' not found after script loaded and 10 tries over 1s"))
                }
            }, 100)
        }
        script.onerror = () => reject(new Error("failed to load script: " + src))
        document.body.appendChild(script)
        setTimeout(() => reject(new Error("script load timeout after 10s: " + src)), 10_000)
    })
}

function addTwitterWidget() {
    return addScript("https://platform.twitter.com/widgets.js", "twttr")
}

function addBskyWidget() {
    return addScript("https://embed.bsky.app/static/embed.js", "bluesky")
}

// @ts-ignore
customElements.define("media-scroller", component(MediaScroller, { useShadowDOM: false }))

/**
 * @param {{createdAt: string|Date}} post
 * @returns {boolean}
 */
function canUpdatePost(post) {
    const d = new Date(post.createdAt)
    // post can only be updated if it was created 15 minutes ago
    return Date.now() - d.getTime() < 15 * 60 * 1000
}

/**
 * @param {URL} url
 */
function findYouTubeID(url) {
    let id = null
    let seconds = null
    if (url.hostname === "www.youtube.com" || url.hostname === "youtube.com" || url.hostname === "youtu.be" || url.hostname === "m.youtube.com" || url.hostname === "music.youtube.com") {
        if (url.pathname === "/watch" && url.searchParams.has("v")) {
            id = decodeURIComponent(url.searchParams.get("v"))
        }

        if (id === null) {
            const parts = url.pathname.split("/")
            if (parts.length === 3 && parts[0] === "" && (parts[1] === "shorts" || parts[1] === "embed" || parts[1] === "live") && parts[2] !== "") {
                id = decodeURIComponent(parts[2])
            }

            if (id === null && parts.length === 2 && parts[0] === "" && parts[1] !== "") {
                id = decodeURIComponent(parts[1])
            }
        }
    }

    if (url.searchParams.has("t")) {
        try {
            const s = decodeURIComponent(url.searchParams.get("t")).replace("s", "")
            seconds = parseInt(s, 10)
        } catch (_) { }
    }

    if (url.searchParams.has("start")) {
        try {
            const s = decodeURIComponent(url.searchParams.get("start"))
            seconds = parseInt(s, 10)
        } catch (_) { }
    }

    return { id, seconds }
}

/**
 * @param {URL} url
 */
function findVimeoID(url) {
    if (url.hostname !== "vimeo.com") {
        return null
    }

    if (url.pathname.match(/^\/\d+$/)) {
        return url.pathname.substring(1)
    }

    if (url.pathname.match(/^\/video\/\d+$/)) {
        return url.pathname.substring(7)
    }

    return null
}

/**
 * @param {URL} url
 * @returns {string|null}
 */
function findTweetID(url) {
    if (!["twitter.com", "x.com", "mobile.twitter.com"].includes(url.hostname)) {
        return null
    }

    const parts = url.pathname.split("/")
    // /{user_handle}/status/{id}
    if (parts.length !== 4 || parts[0] !== "" || parts[1] === "" || parts[2] !== "status" || parts[3] === "") {
        return null
    }

    return parts[3]
}

/**
 * @param {URL} url
 */
function findCoubVideoID(url) {
    if (url.hostname !== "coub.com") {
        return null
    }

    const parts = url.pathname.split("/")
    if (parts.length !== 3 && parts[0] !== "" && parts[1] != "view") {
        return null
    }

    return decodeURIComponent(parts[2])
}

/**
 * @param {URL} url
 */
function findTikTokVideoID(url) {
    // URL example: https://www.tiktok.com/@scout2015/video/6718335390845095173
    if (url.hostname !== "tiktok.com" && !url.hostname.endsWith(".tiktok.com")) {
        return null
    }

    const parts = url.pathname.split("/")
    if (parts.length !== 4) {
        return null
    }

    if (parts[0] !== "" || !parts[1].startsWith("@") || parts[2] !== "video" || parts[3] === "") {
        return null
    }

    return parts[3]
}

const zoom = mediumZoom()

function ZoomableImg({ src, width = undefined, height = undefined }) {
    const imgRef = /** @type {import("lit/directives/ref.js").Ref<HTMLImageElement>} */ (createRef())

    useEffect(() => {
        if (imgRef.value === undefined) {
            return
        }

        const el = /** @type {HTMLImageElement} */ (imgRef.value)
        zoom.attach(el)
        return () => {
            zoom.detach(el)
        }
    }, [imgRef.value])

    return html`<img src="${src}" width="${width}" height="${height}" alt="" loading="lazy" ${ref(imgRef)}>`
}

// @ts-ignore
customElements.define("zoomable-img", component(ZoomableImg, { useShadowDOM: false }))

function togglePostSubscription(postID) {
    return request("POST", `/api/posts/${encodeURIComponent(postID)}/toggle_subscription`)
        .then(resp => resp.body)
}

function toggleReaction(type, resourceID, reaction) {
    const resource = (type === "timeline_item" || type === "post") ? "posts"
        : type === "comment" ? "comments"
            : null
    if (resource === null) {
        return Promise.reject(new Error("unkown resource type " + type))
    }
    return request("POST", `/api/${resource}/${encodeURIComponent(resourceID)}/toggle_reaction`, { body: reaction })
        .then(resp => resp.body)
}

function removeTimelineItem(timelineItemID) {
    return request("DELETE", "/api/timeline/" + encodeURIComponent(timelineItemID))
        .then(() => void 0)
}

function deleteResource(type, resourceID) {
    const resource = (type === "timeline_item" || type === "post") ? "posts"
        : type === "comment" ? "comments"
            : null
    if (resource === null) {
        return Promise.reject(new Error("unkown resource type " + type))
    }
    return request("DELETE", `/api/${resource}/${encodeURIComponent(resourceID)}`)
        .then(() => void 0)
}

/**
 * @param {string} postID
 * @param {FormData|import("../types.js").UpdatePost} body
 * @returns {Promise<import("../types.js").UpdatedPost>}
 */
function updatePost(postID, body) {
    return request("PATCH", "/api/posts/" + encodeURIComponent(postID), { body })
        .then(resp => resp.body)
}

/**
 * @param {string} commentID
 * @param {FormData|import("../types.js").UpdatePost} body
 * @returns {Promise<import("../types.js").UpdatedComment>}
 */
function updateComment(commentID, body) {
    return request("PATCH", "/api/comments/" + encodeURIComponent(commentID), { body })
        .then(resp => resp.body)
}
