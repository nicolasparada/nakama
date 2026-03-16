import { html } from "lit"

/**
 * @typedef {import("../types.js").User} User
 */

/**
 * @param {User} user 
 */
export function Avatar(user) {
    return user.avatarURL !== null ? html`
        <img class="avatar" src="${user.avatarURL}" alt="">
    ` : html`
        <span class="avatar" data-initial="${user.username[0]}"></span>
    `
}
