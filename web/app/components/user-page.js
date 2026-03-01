import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { ifDefined } from "lit/directives/if-defined.js"
import { createRef, ref } from "lit/directives/ref.js"
import { repeat } from "lit/directives/repeat.js"
import { unsafeHTML } from "lit/directives/unsafe-html.js"
import { setLocalAuth } from "../auth.js"
import { authStore, useStore } from "../ctx.js"
import { request } from "../http.js"
import { navigate } from "../router.js"
import { linkify } from "../utils.js"
import { Avatar } from "./avatar.js"
import "./intersectable-comp.js"
import "./post-item.js"
import "./toast-item.js"
import "./user-follow-btn.js"
import "./user-follow-counts.js"

const pageSize = 10

/**
 * @typedef {import("../types.js").TimelineItem} TimelineItem
 */

/**
 * @typedef {import("../types.js").ListTimeline} ListTimeline
 */

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


export default function ({ params }) {
    return html`<user-page .username=${params.username}></user-page>`
}

function UserPage({ username }) {
    const [_, setAuth] = useStore(authStore)
    const [user, setUser] = useState(null)
    const [posts, setPosts] = useState([])
    const [postsEndCursor, setPostsEndCursor] = useState(null)
    const [fetching, setFetching] = useState(user === null)
    const [err, setErr] = useState(null)
    const [loadingMore, setLoadingMore] = useState(false)
    const [noMorePosts, setNoMorePosts] = useState(false)
    const [endReached, setEndReached] = useState(false)
    const [toast, setToast] = useState(null)

    const onPostDeleted = ev => {
        const payload = ev.detail
        setPosts(pp => pp.filter(p => p.id !== payload.id))
    }

    const onUserUpdated = ev => {
        updateUser(ev.detail)
    }

    const onAvatarUpdated = ev => {
        updateUser(ev.detail)
    }

    const onCoverUpdated = ev => {
        updateUser(ev.detail)
    }

    const updateUser = payload => {
        setUser(u => ({
            ...u,
            ...payload,
        }))
        setPosts(pp => pp.map(p => ({
            ...p,
            user: {
                ...p.user,
                ...payload,
            }
        })))
        setAuth(auth => {
            const newAuth = {
                ...auth,
                user: {
                    ...auth.user,
                    ...payload,
                }
            }
            setLocalAuth(newAuth)
            return newAuth
        })
    }

    const loadMore = () => {
        if (loadingMore || noMorePosts) {
            return
        }

        setLoadingMore(true)
        fetchPosts({ username, pageArgs: { after: postsEndCursor } }).then(page => {
            setPosts(pp => [...pp, ...page.items])
            setPostsEndCursor(page.pageInfo.endCursor)

            if (!page.pageInfo.hasNextPage) {
                setNoMorePosts(true)
                setEndReached(true)
            }
        }, err => {
            const msg = "could not fetch more posts: " + err.message
            console.error(msg)
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setLoadingMore(false)
        })
    }

    useEffect(() => {
        setFetching(true)
        Promise.all([
            fetchUser(username),
            fetchPosts({ username, pageArgs: {} }),
        ]).then(([user, postsPage]) => {
            setUser(user)
            setPosts(postsPage.items)
            setPostsEndCursor(postsPage.pageInfo.endCursor)

            if (!postsPage.pageInfo.hasNextPage) {
                setNoMorePosts(true)
            }
        }, err => {
            console.error("could not fetch user and posts:", err)
            if (err.name === "UnauthenticatedError") {
                setAuth(null)
                setLocalAuth(null)
                navigate("/")
            }

            setErr(err)
        }).finally(() => {
            setFetching(false)
        })
    }, [username])

    return html`
        <main class="user-page">
            <div class="user-profile-wrapper" style="${ifDefined(err === null && !fetching && user.coverURL !== null ? `--cover-url: url('${user.coverURL}');` : undefined)}">
                <div class="container">
                    ${err !== null ? html`
                    <p class="error" role="alert">Could not fetch user: ${err.message}</p>
                    ` : fetching ? html`
                    <p class="loader" aria-busy="true" aria-live="polite">Loading user... please wait.</p>
                    ` : html`
                    <user-profile .user=${user} @user-updated=${onUserUpdated} @avatar-updated=${onAvatarUpdated} @cover-updated=${onCoverUpdated}></user-profile>
                    `}
                </div>
            </div>
            <div class="container posts-wrapper">
                <h2>Posts</h2>
                ${err !== null ? html`
                <p class="error" role="alert">Could not fetch posts: ${err.message}</p>
                ` : fetching ? html`
                <p class="loader" aria-busy="true" aria-live="polite">Loading posts... please wait.</p>
                ` : html`
                ${posts.length === 0 ? html`
                <p>0 posts</p>
                ` : html`
                <div class="posts" role="feed">
                    ${repeat(posts, p => p.id, p => html`<post-item .post=${p} .type=${"post"}
                        @resource-deleted=${onPostDeleted}></post-item>`)}
                </div>
                ${!noMorePosts ? html`
                <intersectable-comp @is-intersecting=${loadMore}></intersectable-comp>
                <p class="loader" aria-busy="true" aria-live="polite">Loading posts... please wait.</p>
                ` : endReached ? html`
                <p>End reached.</p>
                ` : null}
                `}
                `}
            </div>
        </main>
        ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
    `
}

// @ts-ignore
customElements.define("user-page", component(UserPage, { useShadowDOM: false }))

function UserProfile({ user: initialUser }) {
    const [auth, setAuth] = useStore(authStore)
    const [user, setUser] = useState(initialUser)
    const [email, setEmail] = useState(user.email)
    const [username, setUsername] = useState(user.username)
    const [bio, setBio] = useState(user.bio ?? "")
    const [waifu, setWaifu] = useState(user.waifu ?? "")
    const [husbando, setHusbando] = useState(user.husbando ?? "")
    const settingsDialogRef = /** @type {import("lit/directives/ref.js").Ref<HTMLDialogElement>} */(createRef())
    const avatarInputRef = /** @type {import("lit/directives/ref.js").Ref<HTMLInputElement>} */(createRef())
    const coverInputRef = /** @type {import("lit/directives/ref.js").Ref<HTMLInputElement>} */(createRef())
    const [sendingMagicLink, setSendingMagicLink] = useState(false)
    const [updatingUser, setUpdatingUser] = useState(false)
    const [updatingAvatar, setUpdatingAvatar] = useState(false)
    const [updatingCover, setUpdatingCover] = useState(false)
    const [theme, setTheme] = useState(() => {
        const value = localStorage.getItem("color-scheme")
        return value !== null ? value : "default"
    })
    const [toast, setToast] = useState(null)

    const dispatchUserUpdated = payload => {
        this.dispatchEvent(new CustomEvent("user-updated", { bubbles: true, detail: payload }))
    }

    const dispatchAvatarUpdated = payload => {
        this.dispatchEvent(new CustomEvent("avatar-updated", { bubbles: true, detail: payload }))
    }

    const dispatchCoverUpdated = payload => {
        this.dispatchEvent(new CustomEvent("cover-updated", { bubbles: true, detail: payload }))
    }

    const onFollowToggle = ev => {
        const payload = ev.detail
        setUser(u => ({
            ...u,
            ...payload,
        }))
    }

    const onSettingsBtnClick = () => {
        if (settingsDialogRef.value !== undefined) {
            settingsDialogRef.value.showModal()
        }
    }

    const onEmailInput = ev => {
        setEmail(ev.currentTarget.value)
    }

    const onUsernameInput = ev => {
        setUsername(ev.currentTarget.value)
    }

    const onUserBioInput = ev => {
        setBio(ev.currentTarget.value)
    }

    const onUserWaifuInput = ev => {
        setWaifu(ev.currentTarget.value)
    }

    const onUserHusbandoInput = ev => {
        setHusbando(ev.currentTarget.value)
    }

    const onEmailFormSubmit = ev => {
        ev.preventDefault()
        if (email === user.email) {
            setToast({ type: "error", content: "same email" })
            return
        }

        const payload = {
            email,
            updateEmail: true,
            redirectURI: location.origin + "/access-callback",
        }
        setSendingMagicLink(true)
        request("POST", "/api/send_magic_link", { body: payload }).then(() => {
            setToast({ type: "success", content: "email verification sended" })
        }, err => {
            const msg = "could not send email verification: " + err.message
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setSendingMagicLink(false)
        })
    }

    const onUserFormSubmit = ev => {
        ev.preventDefault()

        const payload = { username, bio, waifu, husbando }
        for (const [k, v] of Object.entries(payload)) {
            if (v === "") {
                payload[k] = null
            }
        }

        setUpdatingUser(true)
        updateUser(payload).then(() => {
            setAuth(auth => ({
                ...auth,
                user: {
                    ...auth.user,
                    ...payload,
                },
            }))
            setUser(u => ({
                ...u,
                ...payload,
            }))
            setToast({ type: "success", content: "user updated" })
            history.replaceState(history.state, document.title, "/@" + encodeURIComponent(username))
            dispatchUserUpdated(payload)
        }, err => {
            const msg = "could not update user: " + err.message
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setUpdatingUser(false)
        })
    }

    const onAvatarInputChange = ev => {
        const files = ev.currentTarget.files
        if (files === null || files.length !== 1) {
            return
        }

        const avatar = files.item(0)
        submitAvatar(avatar)
    }

    const onAvatarDblClick = () => {
        if (updatingAvatar) {
            return
        }

        avatarInputRef.value.click()
    }

    const onAvatarBtnClick = () => {
        if (avatarInputRef.value === null || updatingAvatar) {
            return
        }

        avatarInputRef.value.click()
    }

    const onAvatarDragOver = ev => {
        ev.preventDefault()
    }

    const onAvatarDrop = ev => {
        ev.preventDefault()
        if (updatingAvatar) {
            return
        }

        const files = ev.dataTransfer.files
        if (!(files instanceof FileList) || files.length !== 1) {
            return
        }

        const avatar = files.item(0)
        submitAvatar(avatar)
    }

    const submitAvatar = avatar => {
        setUpdatingAvatar(true)
        updateAvatar(avatar).then(payload => {
            setAuth(auth => ({
                ...auth,
                user: {
                    ...auth.user,
                    ...payload,
                },
            }))
            setUser(u => ({
                ...u,
                ...payload,
            }))
            setToast({ type: "success", content: "avatar updated" })
            dispatchAvatarUpdated(payload)
        }, err => {
            const msg = "could not update avatar: " + err.message
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setUpdatingAvatar(false)
        })
    }

    const onCoverInputChange = ev => {
        const files = ev.currentTarget.files
        if (files === null || files.length !== 1) {
            return
        }

        const cover = files.item(0)
        submitCover(cover)
    }

    const onCoverDblClick = () => {
        if (coverInputRef.value === undefined || updatingCover) {
            return
        }

        coverInputRef.value.click()
    }

    const onCoverBtnClick = () => {
        if (coverInputRef.value === undefined || updatingCover) {
            return
        }

        coverInputRef.value.click()
    }

    const onCoverDragOver = ev => {
        ev.preventDefault()
    }

    const onCoverDrop = ev => {
        ev.preventDefault()
        if (updatingCover) {
            return
        }

        const files = ev.dataTransfer.files
        if (!(files instanceof FileList) || files.length !== 1) {
            return
        }

        const cover = files.item(0)
        submitCover(cover)
    }

    const submitCover = cover => {
        setUpdatingCover(true)
        updateCover(cover).then(payload => {
            setAuth(auth => ({
                ...auth,
                user: {
                    ...auth.user,
                    ...payload,
                },
            }))
            setUser(u => ({
                ...u,
                ...payload,
            }))
            setToast({ type: "success", content: "cover updated" })
            dispatchCoverUpdated(payload)
        }, err => {
            const msg = "could not update cover: " + err.message
            setToast({ type: "error", content: msg })
        }).finally(() => {
            setUpdatingCover(false)
        })
    }

    const onThemeChange = ev => {
        const value = ev.currentTarget.value
        setTheme(value)

        if (value === "default") {
            localStorage.removeItem("color-scheme")
            document.firstElementChild.removeAttribute("color-scheme")
            return
        }

        localStorage.setItem("color-scheme", value)
        document.firstElementChild.setAttribute("color-scheme", value)
    }

    const onSettingsDialogCloseBtnClick = () => {
        settingsDialogRef.value.close()
    }

    const onSettingsDialogClose = () => {
        setUsername(user.username)
    }

    useEffect(() => {
        if (settingsDialogRef.value === undefined) {
            return
        }

        const el = /** @type {HTMLDialogElement} */ (settingsDialogRef.value)
        if ("HTMLDialogElement" in window && "showModal" in el) {
            return
        }

        import("dialog-polyfill").then(m => m.default).then(dialogPolyfill => {
            if (el !== undefined) {
                dialogPolyfill.registerDialog(el)
            }
        })
    }, [settingsDialogRef.value])

    useEffect(() => {
        setUser(initialUser)
        setUsername(initialUser.username)
    }, [initialUser])

    return html`
        <div class="user-profile">
            <div class="user-details-wrapper">
                <div>
                    <h1>${user.username}</h1>
                    <user-follow-counts .user=${user}></user-follow-counts>
                </div>
                <div class="user-details">
                    ${user.bio !== null && user.bio !== "" ? html`
                        <p>${unsafeHTML(linkify(user.bio))}</p>
                    ` : null}
                    ${(user.waifu !== null && user.waifu !== "") || (user.husbando !== null && user.husbando !== "") ? html`
                        <dl>
                            ${user.waifu !== null && user.waifu !== "" ? html`
                                <dt>Waifu:</dt><dd>${user.waifu}</dd>
                            ` : null}
                            ${user.husbando !== null && user.husbando !== "" ? html`
                                <dt>Husbando:</dt><dd>${user.husbando}</dd>
                            ` : null}
                        </dl>
                    ` : null}
                </div>
            </div>
            ${Avatar(user)}
            <div class="user-controls">
                ${user.me ? html`
                <button @click=${onSettingsBtnClick}>
                    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                        <g data-name="Layer 2">
                            <g data-name="settings">
                                <rect width="24" height="24" opacity="0" />
                                <path
                                    d="M8.61 22a2.25 2.25 0 0 1-1.35-.46L5.19 20a2.37 2.37 0 0 1-.49-3.22 2.06 2.06 0 0 0 .23-1.86l-.06-.16a1.83 1.83 0 0 0-1.12-1.22h-.16a2.34 2.34 0 0 1-1.48-2.94L2.93 8a2.18 2.18 0 0 1 1.12-1.41 2.14 2.14 0 0 1 1.68-.12 1.93 1.93 0 0 0 1.78-.29l.13-.1a1.94 1.94 0 0 0 .73-1.51v-.24A2.32 2.32 0 0 1 10.66 2h2.55a2.26 2.26 0 0 1 1.6.67 2.37 2.37 0 0 1 .68 1.68v.28a1.76 1.76 0 0 0 .69 1.43l.11.08a1.74 1.74 0 0 0 1.59.26l.34-.11A2.26 2.26 0 0 1 21.1 7.8l.79 2.52a2.36 2.36 0 0 1-1.46 2.93l-.2.07A1.89 1.89 0 0 0 19 14.6a2 2 0 0 0 .25 1.65l.26.38a2.38 2.38 0 0 1-.5 3.23L17 21.41a2.24 2.24 0 0 1-3.22-.53l-.12-.17a1.75 1.75 0 0 0-1.5-.78 1.8 1.8 0 0 0-1.43.77l-.23.33A2.25 2.25 0 0 1 9 22a2 2 0 0 1-.39 0zM4.4 11.62a3.83 3.83 0 0 1 2.38 2.5v.12a4 4 0 0 1-.46 3.62.38.38 0 0 0 0 .51L8.47 20a.25.25 0 0 0 .37-.07l.23-.33a3.77 3.77 0 0 1 6.2 0l.12.18a.3.3 0 0 0 .18.12.25.25 0 0 0 .19-.05l2.06-1.56a.36.36 0 0 0 .07-.49l-.26-.38A4 4 0 0 1 17.1 14a3.92 3.92 0 0 1 2.49-2.61l.2-.07a.34.34 0 0 0 .19-.44l-.78-2.49a.35.35 0 0 0-.2-.19.21.21 0 0 0-.19 0l-.34.11a3.74 3.74 0 0 1-3.43-.57L15 7.65a3.76 3.76 0 0 1-1.49-3v-.31a.37.37 0 0 0-.1-.26.31.31 0 0 0-.21-.08h-2.54a.31.31 0 0 0-.29.33v.25a3.9 3.9 0 0 1-1.52 3.09l-.13.1a3.91 3.91 0 0 1-3.63.59.22.22 0 0 0-.14 0 .28.28 0 0 0-.12.15L4 11.12a.36.36 0 0 0 .22.45z"
                                    data-name="&lt;Group&gt;" />
                                <path
                                    d="M12 15.5a3.5 3.5 0 1 1 3.5-3.5 3.5 3.5 0 0 1-3.5 3.5zm0-5a1.5 1.5 0 1 0 1.5 1.5 1.5 1.5 0 0 0-1.5-1.5z" />
                            </g>
                        </g>
                    </svg>
                    <span>Settings</span>
                </button>
                <logout-btn></logout-btn>
                ` : auth !== null ? html`
                <user-follow-btn .user=${user} @follow-toggle=${onFollowToggle}></user-follow-btn>
                ` : null}
            </div>
        </div>
        <dialog class="user-settings-dialog" ${ref(settingsDialogRef)} @close=${onSettingsDialogClose}>
            <div class="user-settings">
                <div class="user-settings-header">
                    <h2>Settings</h2>
                    <button class="close-btn" title="Close" @click=${onSettingsDialogCloseBtnClick}>
                        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                            <g data-name="Layer 2">
                                <g data-name="close">
                                    <rect width="24" height="24" transform="rotate(180 12 12)" opacity="0" />
                                    <path
                                        d="M13.41 12l4.3-4.29a1 1 0 1 0-1.42-1.42L12 10.59l-4.29-4.3a1 1 0 0 0-1.42 1.42l4.3 4.29-4.3 4.29a1 1 0 0 0 0 1.42 1 1 0 0 0 1.42 0l4.29-4.3 4.29 4.3a1 1 0 0 0 1.42 0 1 1 0 0 0 0-1.42z" />
                                </g>
                            </g>
                        </svg>
                    </button>
                </div>

                <form class="update-user-form" @submit=${onUserFormSubmit}>
                    <div class="input-grp">
                        <label for="update-username-input">Username:</label>
                        <input id="update-username-input" type="text" name="username" placeholder="Username" pattern="^[a-zA-Z][a-zA-Z0-9_-]{0,17}$" autocomplete="off"
                            .value=${username}
                            .disabled=${updatingUser}
                            @input=${onUsernameInput}>
                    </div>
                    <div class="input-grp">
                        <label for="update-user-bio-input">Bio:</label>
                        <textarea id="update-user-bio-input" name="bio" placeholder="Bio" autocomplete="off" maxlength="480"
                            .value=${bio}
                            .disabled=${updatingUser}
                            @input=${onUserBioInput}></textarea>
                    </div>
                    <div class="input-grp">
                        <label for="update-user-waifu-input">Waifu:</label>
                        <input id="update-user-waifu-input" type="text" name="waifu" placeholder="Waifu" autocomplete="off" maxlength="32"
                            .value=${waifu}
                            .disabled=${updatingUser}
                            @input=${onUserWaifuInput}>
                    </div>
                    <div class="input-grp">
                        <label for="update-user-husbando-input">Husbando:</label>
                        <input id="update-user-husbando-input" type="text" name="husbando" placeholder="Husbando" autocomplete="off" maxlength="32"
                            .value=${husbando}
                            .disabled=${updatingUser}
                            @input=${onUserHusbandoInput}>
                    </div>
                    <button .disabled=${updatingUser}>Update</button>
                </form>

                <fieldset class="email-fieldset">
                    <form class="update-email-form" @submit="${onEmailFormSubmit}">
                        <div class="input-grp">
                            <label for="update-email-input">Email:</label>
                            <input id="update-email-input" type="email" name="email" placeholder="Email" autocomplete="off"
                                .value=${email}
                                .disabled=${sendingMagicLink}
                                @input=${onEmailInput}>
                        </div>

                        <button .disabled=${sendingMagicLink}>Update and verify</button>
                    </form>
                </fieldset>

                <fieldset class="avatar-fieldset" @drop=${onAvatarDrop} @dragover=${onAvatarDragOver}>
                    <legend>Avatar</legend>
                    <div class="avatar-grp">
                        <div @dblclick=${onAvatarDblClick}>
                            ${Avatar(user)}
                        </div>
                        <input type="file" name="avatar" accept="image/png,image/jpeg" required hidden
                            .disabled=${updatingAvatar} ${ref(avatarInputRef)} @change=${onAvatarInputChange}>
                        <button .disabled=${updatingAvatar} @click=${onAvatarBtnClick}>Update</button>
                    </div>
                </fieldset>
                <fieldset class="cover-fieldset" @drop=${onCoverDrop} @dragover=${onCoverDragOver}>
                    <legend>Cover</legend>
                    <div class="cover-grp">
                        ${user.coverURL !== null ? html`
                            <img src="${user.coverURL}" @dblclick=${onCoverDblClick}>
                        ` : null}
                        <input type="file" name="cover" accept="image/png,image/jpeg" required hidden
                            .disabled=${updatingCover} ${ref(coverInputRef)} @change=${onCoverInputChange}>
                        <button .disabled=${updatingCover} @click=${onCoverBtnClick}>Update</button>
                    </div>
                </fieldset>
                <fieldset class="theme-fieldset">
                    <legend>Theme</legend>
                    <label>
                        <input type="radio" name="theme" value="default" .checked=${theme === "default"} @change=${onThemeChange}>
                        <span>Default</span>
                    </label>
                    <label>
                        <input type="radio" name="theme" value="dark" .checked=${theme === "dark"} @change=${onThemeChange}>
                        <span>Dark</span>
                    </label>
                    <label>
                        <input type="radio" name="theme" value="light" .checked=${theme === "light"} @change=${onThemeChange}>
                        <span>Light</span>
                    </label>
                </fieldset>
            </div>
            ${toast !== null ? html`<toast-item .toast=${toast}></toast-item>` : null}
        </dialog>
    `
}

// @ts-ignore
customElements.define("user-profile", component(UserProfile, { useShadowDOM: false }))

function LogoutBtn() {
    const [, setAuth] = useStore(authStore)

    const onClick = () => {
        localStorage.removeItem("auth")
        setAuth(null)
        navigate("/")
    }

    return html`
        <button @click=${onClick}>
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                <g data-name="Layer 2">
                    <g data-name="log-out">
                        <rect width="24" height="24" transform="rotate(90 12 12)" opacity="0" />
                        <path d="M7 6a1 1 0 0 0 0-2H5a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h2a1 1 0 0 0 0-2H6V6z" />
                        <path
                            d="M20.82 11.42l-2.82-4a1 1 0 0 0-1.39-.24 1 1 0 0 0-.24 1.4L18.09 11H10a1 1 0 0 0 0 2h8l-1.8 2.4a1 1 0 0 0 .2 1.4 1 1 0 0 0 .6.2 1 1 0 0 0 .8-.4l3-4a1 1 0 0 0 .02-1.18z" />
                    </g>
                </g>
            </svg>
            <span>Logout</span>
        </button>
    `
}

customElements.define("logout-btn", component(LogoutBtn, { useShadowDOM: false }))

/**
 * @param {string} username
 */
function fetchUser(username) {
    return request("GET", "/api/users/" + encodeURIComponent(username))
        .then(resp => resp.body)
}

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

/**
 * @param {{username?:string,bio?:string,waifu?:string,husbando?:string}} payload
 */
function updateUser({ username, bio, waifu, husbando }) {
    return request("PATCH", "/api/auth_user", { body: { username, bio, waifu, husbando } })
}

/**
 * @param {File} avatar
 */
function updateAvatar(avatar) {
    return request("PUT", "/api/auth_user/avatar", { body: avatar })
        .then(resp => resp.body)
        .then(avatarURL => ({ avatarURL }))
}

/**
 * @param {File} cover
 */
function updateCover(cover) {
    return request("PUT", "/api/auth_user/cover", { body: cover })
        .then(resp => resp.body)
        .then(coverURL => ({ coverURL }))
}
