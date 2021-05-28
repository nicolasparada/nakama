/**
 * @typedef User
 * @property {string=} id
 * @property {string} username
 * @property {string=} avatarURL
 */

/**
 * @typedef UserProfile
 * @property {string=} id
 * @property {string=} email
 * @property {string} username
 * @property {string=} avatarURL
 * @property {number} followersCount
 * @property {number} followeesCount
 * @property {boolean} me
 * @property {boolean} following
 * @property {boolean} followeed
 */

/**
 * @typedef DevLoginOutput
 * @property {string} token
 * @property {string|Date} expiresAt
 * @property {User} user
 */

/**
 * @typedef {object} ReactionCount
 * @property {"emoji"|"custom"} type
 * @property {string} reaction
 * @property {number} count
 * @property {boolean} reacted
 */

/**
 * @typedef Post
 * @property {string} id
 * @property {string} content
 * @property {boolean} NSFW
 * @property {string=} spoilerOf
 * @property {number} likesCount
 * @property {ReactionCount[]} reactionCounts
 * @property {number} commentsCount
 * @property {string|Date} createdAt
 * @property {User=} user
 * @property {boolean} mine
 * @property {boolean} liked
 * @property {boolean} subscribed
 */

/**
 * @typedef TimelineItem
 * @property {string} id
 * @property {Post=} post
 */

/**
 * @template T
 * @typedef Page
 * @property {T[]} items
 * @property {string|null} startCursor
 * @property {string|null} endCursor
 */

/**
 * @typedef Comment
 * @property {string} id
 * @property {string} content
 * @property {number} likesCount
 * @property {string|Date} createdAt
 * @property {User=} user
 * @property {boolean} mine
 * @property {boolean} liked
 */

/**
 * @typedef CreatePostInput
 * @property {string} content
 * @property {boolean=} NSFW
 * @property {string=} spoilerOf
 */

/**
 * @typedef ToggleFollowOutput
 * @property {number} followersCount
 * @property {boolean} following
 */

/**
 * @typedef ToggleLikeOutput
 * @property {number} likesCount
 * @property {boolean} liked
 */

/**
 * @typedef ToggleSubscriptionOutput
 * @property {boolean} subscribed
 */

/**
 * @typedef Notification
 * @property {string} id
 * @property {string[]} actors
 * @property {"follow"|"comment"|"post_mention"|"comment_mention"} type
 * @property {string=} postID
 * @property {boolean} read
 * @property {string|Date} issuedAt
 */

export default undefined
