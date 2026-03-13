import { component, useEffect, useState } from "haunted"
import { html } from "lit"
import { ifDefined } from "lit/directives/if-defined.js"
import { authStore, hasUnreadNotificationsStore, notificationsEnabledStore, useStore } from "../ctx.js"
import { request, subscribe } from "../http.js"
import { Avatar } from "./avatar.js"

const rePostPagePath = /^\/posts\/(?<postID>[^\/]+)$/

/**
 * @typedef {import("../types.js").AppNotification} AppNotification
 */

function AppHeader() {
    const [auth] = useStore(authStore)
    const [hasUnreadNotifications, setHasUnreadNotifications] = useStore(hasUnreadNotificationsStore)
    const [notificationsEnabled] = useStore(notificationsEnabledStore)
    const [activePath, setActivePath] = useState(location.pathname)

    const onNewNotificationArrive = n => {
        if (viewingNotificationPage(n)) {
            markNotificationAsRead(n.id).then(() => {
                n.read = true
                dispatchNewNotificationArrived(n)
            }, err => {
                console.error("could not mark arriving notification as read:", err)
            })
            return
        }

        setHasUnreadNotifications(true)
        dispatchNewNotificationArrived(n)

        if (notificationsEnabled) {
            showNotification(n)
        }
    }

    const onLinkClick = ev => {
        if (location.href === ev.currentTarget.href) {
            document.documentElement.scrollTop = 0
        }
    }

    const onNavigation = () => {
        setActivePath(location.pathname)
    }

    const onNotificationRead = () => {
        fetchHasUnreadNotifications().then(v => {
            setHasUnreadNotifications(v)
        }, err => {
            console.error("could not fetch has unread notifications:", err)
        })
    }

    useEffect(() => {
        if (auth === null) {
            return
        }

        fetchHasUnreadNotifications().then(v => {
            setHasUnreadNotifications(v)
        }, err => {
            console.error("could not fetch has unread notifications:", err)
        })

        return subscribeToNotifications(onNewNotificationArrive)
    }, [auth])

    useEffect(() => {
        if (!("setAppBadge" in navigator && "clearAppBadge" in navigator)) {
            return
        }

        if (hasUnreadNotifications) {
            navigator["setAppBadge"]()
            return
        }

        navigator["clearAppBadge"]()
    }, [hasUnreadNotifications])

    useEffect(() => {
        addEventListener("notification-read", onNotificationRead)
        addEventListener("popstate", onNavigation)
        addEventListener("pushstate", onNavigation)
        addEventListener("replacestate", onNavigation)
        addEventListener("hashchange", onNavigation)
        return () => {
            removeEventListener("popstate", onNavigation)
            removeEventListener("pushstate", onNavigation)
            removeEventListener("replacestate", onNavigation)
            removeEventListener("hashchange", onNavigation)
            removeEventListener("notification-read", onNotificationRead)
        }
    }, [])

    const isCurrentPage = pathname => ifDefined(activePath === pathname ? "page" : undefined)

    return html`
        <header>
            <nav class="container">
                <ul>
                    <li>
                        <a href="/" class="btn" title="Home" aria-current="${isCurrentPage("/")}" @click=${onLinkClick}>
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                                <g data-name="Layer 2">
                                    <g data-name="home">
                                        <rect width="24" height="24" opacity="0" />
                                        <path
                                            d="M20.42 10.18L12.71 2.3a1 1 0 0 0-1.42 0l-7.71 7.89A2 2 0 0 0 3 11.62V20a2 2 0 0 0 1.89 2h14.22A2 2 0 0 0 21 20v-8.38a2.07 2.07 0 0 0-.58-1.44zM10 20v-6h4v6zm9 0h-3v-7a1 1 0 0 0-1-1H9a1 1 0 0 0-1 1v7H5v-8.42l7-7.15 7 7.19z" />
                                    </g>
                                </g>
                            </svg>
                        </a>
                    </li>
                    ${auth !== null ? html`
                    <li>
                        <a href="/notifications" class="btn${hasUnreadNotifications ? " has-unread-notifications" : ""}"
                            title="Notifications" aria-current="${isCurrentPage("/notifications")}" @click=${onLinkClick}>
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                                <g data-name="Layer 2">
                                    <g data-name="bell">
                                        <rect width="24" height="24" opacity="0" />
                                        <path
                                            d="M20.52 15.21l-1.8-1.81V8.94a6.86 6.86 0 0 0-5.82-6.88 6.74 6.74 0 0 0-7.62 6.67v4.67l-1.8 1.81A1.64 1.64 0 0 0 4.64 18H8v.34A3.84 3.84 0 0 0 12 22a3.84 3.84 0 0 0 4-3.66V18h3.36a1.64 1.64 0 0 0 1.16-2.79zM14 18.34A1.88 1.88 0 0 1 12 20a1.88 1.88 0 0 1-2-1.66V18h4zM5.51 16l1.18-1.18a2 2 0 0 0 .59-1.42V8.73A4.73 4.73 0 0 1 8.9 5.17 4.67 4.67 0 0 1 12.64 4a4.86 4.86 0 0 1 4.08 4.9v4.5a2 2 0 0 0 .58 1.42L18.49 16z" />
                                    </g>
                                </g>
                            </svg>
                        </a>
                    </li>
                    ` : null}
                    <li>
                        <a href="/search" class="btn" title="Search" aria-current="${isCurrentPage("/search")}"
                            @click=${onLinkClick}>
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                                <g data-name="Layer 2">
                                    <g data-name="search">
                                        <rect width="24" height="24" opacity="0" />
                                        <path
                                            d="M20.71 19.29l-3.4-3.39A7.92 7.92 0 0 0 19 11a8 8 0 1 0-8 8 7.92 7.92 0 0 0 4.9-1.69l3.39 3.4a1 1 0 0 0 1.42 0 1 1 0 0 0 0-1.42zM5 11a6 6 0 1 1 6 6 6 6 0 0 1-6-6z" />
                                    </g>
                                </g>
                            </svg>
                        </a>
                    </li>
                    ${auth !== null ? html`
                    <li class="profile-link-item">
                        <a href="/@${auth.user.username}" class="btn profile-link" title="Profile"
                            aria-current="${isCurrentPage("/@" + auth.user.username)}" @click=${onLinkClick}>
                            ${Avatar(auth.user)}
                        </a>
                    </li>
                    ` : null}
                </ul>
            </nav>
        </header>
    `
}

customElements.define("app-header", component(AppHeader, { useShadowDOM: false }))

function showNotification(n) {
    const title = notificationTitle(n)
    const body = notificationBody(n)
    const sysn = new Notification(title, {
        body,
        tag: n.id,
        data: n,
        icon: location.origin + "/icons/logo-circle-512.png",
    })
    const onSysnClick = ev => {
        ev.preventDefault()

        window.open(location.origin + notificationPathname(n))
        sysn.close()
        markNotificationAsRead(n.id).then(() => {
            dispatchEvent(new CustomEvent("notification-read", { bubbles: true, detail: n }))
        })
    }

    sysn.addEventListener("click", onSysnClick, { once: true })
}

/**
 * @param {AppNotification} n 
 * @returns {string}
 */
function notificationTitle(n) {
    switch (n.type) {
        case "follow":
            return "New follow"
        case "comment":
            return "New commented"
        case "post_mention":
            return "New post mention"
        case "comment_mention":
            return "New comment mention"
        default:
            return "New notification"
    }
}

/**
 * @param {AppNotification} n 
 * @returns {string}
 */
function notificationBody(n) {
    const getActors = () => {
        const aa = n.actorUsernames
        switch (aa.length) {
            case 0:
                return "Someone"
            case 1:
                return aa[0]
            case 2:
                return `${aa[0]} and ${aa[1]}`
            default:
                return `${aa[0]} and ${aa.length - 1} others`
        }
    }

    const getAction = () => {
        switch (n.type) {
            case "follow":
                return "followed you"
            case "comment":
                return "commented in a post"
            case "post_mention":
                return "mentioned you in a post"
            case "comment_mention":
                return "mentioned you in a comment"
            default:
                return "did something"
        }
    }

    return getActors() + " " + getAction()
}

/**
 * @param {AppNotification} n 
 * @returns {string}
 */
function notificationPathname(n) {
    if (typeof n.postID === "string" && n.postID !== "") {
        return "/posts/" + encodeURIComponent(n.postID)
    }

    if (n.type === "follow") {
        return "/@" + encodeURIComponent(n.actorUsernames[0])
    }

    return "/notifications"
}

/**
 * @param {AppNotification} n 
 * @returns {boolean}
 */
function viewingNotificationPage(n) {
    if (!document.hasFocus()) {
        return false
    }

    const postPageMatch = rePostPagePath.exec(location.pathname)
    if (postPageMatch === null) {
        return false
    }

    const postID = decodeURIComponent(postPageMatch.groups.postID)
    return postID === n.postID
}

function fetchHasUnreadNotifications() {
    return request("GET", "/api/has_unread_notifications")
        .then(resp => resp.body)
        .then(v => Boolean(v))
}

/**
 * @param {(Notification) => any} cb 
 */
function subscribeToNotifications(cb) {
    return subscribe("/api/notifications", n => {
        n.issuedAt = new Date(n.issuedAt)
        cb(n)
    })
}

/**
 * @param {AppNotification} n 
 */
function dispatchNewNotificationArrived(n) {
    dispatchEvent(new CustomEvent("new-notification-arrived", { bubbles: true, detail: n }))
}

function markNotificationAsRead(notificationID) {
    return request("POST", `/api/notifications/${encodeURIComponent(notificationID)}/mark_as_read`)
        .then(() => void 0)
}
