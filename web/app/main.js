import { component, render, useEffect, useState } from "haunted"
import { html } from "lit"
import { registerTranslateConfig, translate, use as useLang } from "lit-translate"
import { until } from "lit/directives/until.js"
import { setLocalAuth } from "./auth.js"
import "./components/app-header.js"
import { authStore, useStore } from "./ctx.js"
import { request } from "./http.js"
import { createRouter, hijackClicks } from "./router.js"

const router = createRouter()
router.route("/", guardView(view("home")))
router.route("/access-callback", view("access-callback"))
router.route("/search", view("search"))
router.route("/notifications", guardView(view("notifications")))
router.route("/privacy-policy", view("privacy-policy"))
router.route(/^\/posts\/(?<postID>[^\/]+)$/, view("post"))
router.route(/^\/tagged-posts\/(?<tag>[^\/]+)$/, view("tagged-posts"))
router.route(/^\/@(?<username>[^\/]+)$/, view("user"))
router.route(/^\/@(?<username>[^\/]+)\/followees$/, view("user-followees"))
router.route(/^\/@(?<username>[^\/]+)\/followers$/, view("user-followers"))
router.route(/.*/, view("not-found"))

addEventListener("click", hijackClicks)

function view(name) {
    return params => html`${until(import(`./components/${name}-page.js`).then(m => m.default({ params }), err => {
        console.error("could not import page:", err)
        return html`
            <div class="container">
                <p class="error">${translate("errView")}</p>
            </div>
        `
    }), PageLoader())}`
}

function GuardedView({ args, component, fallback }) {
    const [auth] = useStore(authStore)

    return auth !== null ? component(...args) : fallback(...args)
}

// @ts-ignore
customElements.define("guarded-view", component(GuardedView, { useShadowDOM: false }))

/**
 * @param {function} component
 */
function guardView(component, fallback = view("access")) {
    return (...args) => {
        return html`<guarded-view .args=${args} .component=${component} .fallback=${fallback}>`
    }
}

function PageLoader() {
    return html`
        <main class="container loader" aria-busy="true" aria-live="polite">
            <p>${translate("pageLoader")}</p>
        </main>
    `
}

function RouterView({ router }) {
    const [view, setView] = useState(router.exec())

    const onPopState = () => {
        setView(router.exec())
    }

    useEffect(() => {
        addEventListener("popstate", onPopState)
        addEventListener("pushstate", onPopState)
        addEventListener("replacestate", onPopState)
        addEventListener("hashchange", onPopState)
        return () => {
            removeEventListener("popstate", onPopState)
            removeEventListener("pushstate", onPopState)
            removeEventListener("replacestate", onPopState)
            removeEventListener("hashchange", onPopState)
        }
    }, [])

    return view
}

// @ts-ignore
customElements.define("router-view", component(RouterView, { useShadowDOM: false }))

const oneDayInMs = 1000 * 60 * 60 * 24
const sevenDaysInMs = 1000 * 60 * 60 * 24

function NakamaApp() {
    const [auth, setAuth] = useStore(authStore)

    const tryRefreshAuth = () => {
        if (auth === null) {
            return
        }

        const inSevenDays = new Date()
        inSevenDays.setTime(inSevenDays.getTime() + sevenDaysInMs)

        if (auth.expiresAt >= inSevenDays) {
            return
        }

        fetchToken().then(payload => {
            setAuth(auth => {
                const newAuth = {
                    ...auth,
                    ...payload,
                }
                setLocalAuth(newAuth)
                return newAuth
            })
        }, err => {
            console.error("could not refresh auth:", err)
        })
    }

    useEffect(() => {
        if (auth === null) {
            return
        }

        tryRefreshAuth()

        const id = setInterval(() => {
            tryRefreshAuth()
        }, oneDayInMs)

        return () => {
            clearInterval(id)
        }
    }, [auth])

    return html`
        <app-header></app-header>
        <router-view .router=${router}></router-view>
    `
}

customElements.define("nakama-app", component(NakamaApp, { useShadowDOM: false }))

registerTranslateConfig({
    loader: lang => fetch(`/i18n/${lang}.json`).then(res => res.json()),
})

const lang = detectLang()
document.documentElement.lang = lang
useLang(lang).then(() => {
    render(html`<nakama-app></nakama-app>`, document.body)
})

function fetchToken() {
    return request("GET", "/api/token")
        .then(resp => resp.body)
        .then(auth => {
            auth.expiresAt = new Date(auth.expiresAt)
            return auth
        })
}

function detectLang() {
    let lang = localStorage.getItem("preferred_lang")
    if (lang === "es") {
        return "es"
    }

    if (Array.isArray(window.navigator.languages)) {
        for (const lang of window.navigator.languages) {
            if (lang === "es" || (typeof lang === "string" && lang.startsWith("es-"))) {
                return "es"
            }
        }
    }

    lang = window.navigator["userLanguage"]
    if (lang === "es" || (typeof lang === "string" && lang.startsWith("es-"))) {
        return "es"
    }


    lang = window.navigator.language
    if (lang === "es" || (typeof lang === "string" && lang.startsWith("es-"))) {
        return "es"
    }

    return "en"
}

if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js")
    navigator.serviceWorker.addEventListener("message", ev => {
        if (typeof ev.data !== "object"
            || ev.data === null
            || ev.data.type !== "notificationclick"
            || typeof ev.data.detail !== "object"
            || ev.data.detail === null
            || typeof ev.data.detail.id !== "string") {
            return
        }

        const n = ev.data.detail
        markNotificationAsRead(n.id).then(() => {
            dispatchEvent(new CustomEvent("notification-read", { bubbles: true, detail: n }))
        })
    })
}

addEventListener("error", onError)
addEventListener("unhandledrejection", onUnHandledRejection)

/**
 * @param {ErrorEvent} ev
 */
function onError(ev) {
    if (ev.error instanceof DOMException && ev.error.name === "AbortError") {
        return
    }

    let errLog = ""
    if (typeof ev.error === "object" && ev.error !== null && typeof ev.error.stack === "string") {
        errLog = ev.error.stack + ": " + ev.error.message
    } else {
        errLog = String(ev.error)
    }

    pushLog(errLog).catch(err => {
        console.error("could not push error log:", err)
    })
}

/**
 * @param {PromiseRejectionEvent} ev
 */
function onUnHandledRejection(ev) {
    if (ev.reason instanceof DOMException && ev.reason.name === "AbortError") {
        return
    }

    let errLog = ""
    if (typeof ev.reason === "object" && ev.reason !== null && typeof ev.reason.stack === "string") {
        errLog = ev.reason.stack + ": " + ev.reason.message
    } else {
        errLog = String(ev.reason)
    }

    pushLog(errLog).catch(err => {
        console.error("could not push unhandled rejection log:", err)
    })
}

function pushLog(err) {
    return request("POST", "/api/logs", { body: { error: err } })
}

function markNotificationAsRead(notificationID) {
    return request("POST", `/api/notifications/${encodeURIComponent(notificationID)}/mark_as_read`)
        .then(() => void 0)
}

const now = new Date()
const isHalloween = now.getMonth() + 1 === 10 && now.getDate() === 31
if (isHalloween) {
    import("./halloween.js").then(({ initHalloween }) => {
        initHalloween()
    })
    // document.documentElement.classList.add("halloween")
}

const isChristmas = now.getMonth() + 1 === 12
if (isChristmas) {
    animateImage("/images/padoru.gif")
}

const isAnniversary = now.getMonth() + 1 === 5 && now.getDate() === 4
if (isAnniversary) {
    animateImage("/images/trunks_party.gif")
}

/**
 * @param {string} src
 */
function animateImage(src) {
    const img = document.createElement("img")
    img.src = src
    img.id = "animateimage"
    const animation = document.createElement("style")
    animation.textContent = `
        @keyframes animateimage {
            to {
                transform: translateX(calc(100vw + 3rem));
            }
        }
    `
    img.style.cssText = `
        --y: 50%;
        --size: 3rem;

        width: var(--size);
        height: var(--size);
        position: fixed;
        top: calc(var(--y) + var(--size));

        user-select: none;
        touch-action: none;

        animation: animateimage linear 5s;
    `
    img.addEventListener("load", () => {
        document.head.appendChild(animation)
        document.body.appendChild(img)

        img.addEventListener("animationend", () => {
            img.style.animationName = "none"
            img.style.setProperty("--y", randNumberBetween(5, 95) + "%")
            requestAnimationFrame(() => {
                img.offsetHeight
                img.style.animationName = "animateimage"
            })
        })
    }, { once: true })
}

function randNumberBetween(min, max) {
    return Math.floor(Math.random() * (max - min + 1) + min)
}
