import { getAuthUser, isAuthenticated } from "../auth.js"
import { doGet, doPost, subscribe } from "../http.js"
import { ago, escapeHTML, linkify, smartTrim, replaceNode, el } from "../utils.js"
import renderAvatarHTML from "./avatar.js"
import { heartIconSVG, heartOulineIconSVG } from "./icons.js"
import renderList from "./list.js"
import renderPost from "./post.js"

const PAGE_SIZE = 3

const template = document.createElement("template")
template.innerHTML = `
    <div class="post-wrapper">
        <div class="container">
            <div id="post-outlet"></div>
        </div>
    </div>
    <div class="container">
        <div id="comments-outlet" class="comments-wrapper"></div>
        <form id="comment-form" class="comment-form" hidden>
            <textarea name="content" placeholder="Say something..." maxlength="480" required></textarea>
            <button class="comment-form-button" hidden>
                <svg class="icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="paper-plane"><rect width="24" height="24" opacity="0"/><path d="M21 4a1.31 1.31 0 0 0-.06-.27v-.09a1 1 0 0 0-.2-.3 1 1 0 0 0-.29-.19h-.09a.86.86 0 0 0-.31-.15H20a1 1 0 0 0-.3 0l-18 6a1 1 0 0 0 0 1.9l8.53 2.84 2.84 8.53a1 1 0 0 0 1.9 0l6-18A1 1 0 0 0 21 4zm-4.7 2.29l-5.57 5.57L5.16 10zM14 18.84l-1.86-5.57 5.57-5.57z"/></g></g></svg>
                <span>Comment</span>
            </button>
        </form>
    </div>
`

export default async function renderPostPage(params) {
    const postID = BigInt(params.postID)
    const [post, comments] = await Promise.all([
        fetchPost(postID),
        fetchComments(postID),
    ])

    const list = renderList({
        items: comments,
        renderItem: renderComment,
        loadMoreFunc: before => fetchComments(post.id, before),
        pageSize: PAGE_SIZE,
        reverse: true,
    })

    const page = /** @type {DocumentFragment} */ (template.content.cloneNode(true))
    const postOutlet = page.getElementById("post-outlet")
    let commentsLink = /** @type {HTMLAnchorElement} */ (null)
    let commentsCountEl = /** @type {HTMLElement=} */ (null)
    const commentsOutlet = page.getElementById("comments-outlet")
    const commentForm = /** @type {HTMLFormElement} */ (page.getElementById("comment-form"))
    const commentFormTextArea = commentForm.querySelector("textarea")
    const commentFormButton = commentForm.querySelector("button")
    let initialPostFormTextAreaHeight = /** @type {string=} */ (undefined)

    const incrementCommentsCount = () => {
        if (commentsLink === null) {
            commentsLink = postOutlet.querySelector(".comments-link")
        }
        if (commentsCountEl === null) {
            commentsCountEl = postOutlet.querySelector(".comments-count")
        }
        post.commentsCount++
        commentsLink.setAttribute("aria-title", post.commentsCount + " comments")
        commentsCountEl.textContent = String(post.commentsCount)
    }

    /**
     * @param {Event} ev
     */
    const onCommentFormSubmit = async ev => {
        ev.preventDefault()
        const content = smartTrim(commentFormTextArea.value)
        if (content === "") {
            commentFormTextArea.setCustomValidity("Empty")
            commentFormTextArea.reportValidity()
            return
        }

        commentFormTextArea.disabled = true
        commentFormButton.disabled = true
        try {
            const comment = await createComment(post.id, content)

            list.enqueue(comment)
            list.flush()
            incrementCommentsCount()

            commentForm.reset()
        } catch (err) {
            console.error(err)
            alert(err.message)
            setTimeout(() => {
                commentFormTextArea.focus()
            })
        } finally {
            commentFormTextArea.disabled = false
            commentFormButton.disabled = false
        }
    }

    const onCommentFormTextAreaInput = () => {
        commentFormTextArea.setCustomValidity("")
        commentFormButton.hidden = smartTrim(commentFormTextArea.value) === ""
        if (initialPostFormTextAreaHeight === undefined) {
            initialPostFormTextAreaHeight = commentFormTextArea.style.height
        }
        commentFormTextArea.style.height = initialPostFormTextAreaHeight
        commentFormTextArea.style.height = commentFormTextArea.scrollHeight + "px"
    }

    /**
     * @param {import("../types.js").Comment} comment
     */
    const onCommentArrive = comment => {
        list.enqueue(comment)
        incrementCommentsCount()
    }

    const unsubscribeFromComments = subscribeToComments(post.id, onCommentArrive)

    const onPageDisconnect = () => {
        unsubscribeFromComments()
        list.teardown()
    }

    postOutlet.appendChild(renderPost(post))
    commentsOutlet.appendChild(list.el)
    if (isAuthenticated()) {
        commentForm.hidden = false
        commentForm.addEventListener("submit", onCommentFormSubmit)
        commentFormTextArea.addEventListener("input", onCommentFormTextAreaInput)
    } else {
        commentForm.remove()
    }
    page.addEventListener("disconnect", onPageDisconnect)

    return page
}

/**
 * @param {import("../types.js").Comment} comment
 */
function renderComment(comment) {
    const authenticated = isAuthenticated()
    const { user } = comment
    const content = linkify(escapeHTML(comment.content))

    const article = document.createElement("article")
    article.className = "card micro-post"
    article.setAttribute("aria-label", `${user.username}'s comment`)
    article.innerHTML = `
        <div class="micro-post-header">
            <a class="micro-post-user" href="/users/${user.username}">
                ${renderAvatarHTML(user)}
                <span>${user.username}</span>
            </a>
            <time class="micro-post-ts" datetime="${comment.createdAt}">${ago(comment.createdAt)}</time>
        </div>
        <div class="micro-post-content">
            <p>${content}</p>
        </div>
        <div class="micro-post-controls">
            ${authenticated ? `
                <button class="like-button"
                    title="${comment.liked ? "Unlike" : "Like"}"
                    aria-pressed="${comment.liked}"
                    aria-label="${comment.likesCount} likes">
                    <span class="likes-count">${comment.likesCount}</span>
                    ${comment.liked ? heartIconSVG : heartOulineIconSVG}
                </button>
            ` : `
                <span class="likes-count-wrapper" aria-label="${comment.likesCount} likes">
                    <span>${comment.likesCount}</span>
                    ${heartOulineIconSVG}
                </span>
            `}
            ${comment.mine ? `
                <button title="More">
                    <svg class="icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g data-name="Layer 2"><g data-name="more-horizotnal"><rect width="24" height="24" opacity="0"/><circle cx="12" cy="12" r="2"/><circle cx="19" cy="12" r="2"/><circle cx="5" cy="12" r="2"/></g></g></svg>
                </button>
            ` : ""}
        </div>
    `

    const likeButton = /** @type {HTMLButtonElement=} */ (article.querySelector(".like-button"))
    if (likeButton !== null) {
        const likesCountEl = likeButton.querySelector(".likes-count")

        const onLikeButtonClick = async () => {
            likeButton.disabled = true
            try {
                const out = await toggleCommentLike(comment.id)

                comment.likesCount = out.likesCount
                comment.liked = out.liked

                likeButton.title = out.liked ? "Unlike" : "Like"
                likeButton.setAttribute("aria-pressed", String(out.liked))
                likeButton.setAttribute("aria-label", out.likesCount + " likes")
                replaceNode(
                    likeButton.querySelector("svg"),
                    el(out.liked ? heartIconSVG : heartOulineIconSVG),
                )
                likesCountEl.textContent = String(out.likesCount)
            } catch (err) {
                console.error(err)
                alert(err.message)
            } finally {
                likeButton.disabled = false
            }
        }

        likeButton.addEventListener("click", onLikeButtonClick)
    }

    return article
}

/**
 * @param {bigint} postID
 * @returns {Promise<import("../types.js").Post>}
 */
function fetchPost(postID) {
    return doGet("/api/posts/" + postID)
}

/**
 * @param {bigint} postID
 * @param {bigint=} before
 * @returns {Promise<import("../types.js").Comment[]>}
 */
function fetchComments(postID, before = 0n) {
    return doGet(`/api/posts/${postID}/comments?before=${before}&last=${PAGE_SIZE}`)
}

/**
 * @param {bigint} postID
 * @param {string} content
 * @returns {Promise<import("../types.js").Comment>}
 */
async function createComment(postID, content) {
    const comment = await doPost(`/api/posts/${postID}/comments`, { content })
    comment.user = getAuthUser()
    return comment
}

/**
 *
 * @param {bigint} postID
 * @param {function(import("../types.js").Comment):any} cb
 */
function subscribeToComments(postID, cb) {
    return subscribe(`/api/posts/${postID}/comments`, cb)
}

/**
 * @param {bigint} commentID
 * @returns {Promise<import("../types.js").ToggleLikeOutput>}
 */
function toggleCommentLike(commentID) {
    return doPost(`/api/comments/${commentID}/toggle_like`)
}
