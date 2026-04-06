/** @typedef {import("./types.js").AuthState} AuthState */
/** @typedef {import("./types.js").AuthUser} AuthUser */

export function getLocalAuth() {
    const authItem = localStorage.getItem("auth")
    if (authItem === null) {
        return null
    }

    let auth
    try {
        auth = JSON.parse(authItem)
    } catch (_) {
        return null
    }

    return normalizeAuth(auth)
}

/** @param {AuthState|((auth: AuthState|null) => AuthState|null)|null} auth */
export function setLocalAuth(auth) {
    const newAuth = typeof auth === "function" ? auth(getLocalAuth()) : auth
    const normalizedAuth = newAuth === null ? null : normalizeAuth(newAuth)
    if (normalizedAuth === null) {
        localStorage.removeItem("auth")
        return
    }

    try {
        localStorage.setItem("auth", JSON.stringify(normalizedAuth))
    } catch (_) {}
}

/** @param {unknown} user */
export function authFromUser(user) {
    const normalizedUser = normalizeUser(user)
    if (normalizedUser === null) {
        throw new TypeError("invalid auth user")
    }

    return {
        user: normalizedUser,
    }
}

/** @param {unknown} auth */
function normalizeAuth(auth) {
    const record = /** @type {{ user?: unknown }|null} */ (auth)
    if (typeof record !== "object" || record === null || typeof record.user !== "object" || record.user === null) {
        return null
    }

    const user = normalizeUser(record.user)
    if (user === null) {
        return null
    }

    return {
        user,
    }
}

/** @param {unknown} user */
function normalizeUser(user) {
    const record = /** @type {{ id?: unknown, username?: unknown, avatarURL?: unknown, avatar_url?: unknown }|null} */ (
        user
    )
    if (
        typeof record !== "object" ||
        record === null ||
        typeof record.id !== "string" ||
        typeof record.username !== "string" ||
        !(typeof record.avatarURL === "string" || record.avatarURL === null || typeof record.avatar_url === "string")
    ) {
        return null
    }

    /** @type {AuthUser} */
    return {
        id: record.id,
        username: record.username,
        avatarURL:
            typeof record.avatarURL === "string"
                ? record.avatarURL
                : typeof record.avatar_url === "string"
                  ? record.avatar_url
                  : null,
    }
}
