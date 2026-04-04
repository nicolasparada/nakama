import "linkify-plugin-hashtag"
import "linkify-plugin-mention"
import linkifyString from "linkify-string"
import { find as findURLs } from "linkifyjs"

/**
 * @param {string} s
 */
export function linkify(s) {
    return linkifyString(s, {
        truncate: 42,
        defaultProtocol: "https",
        target: (_, type) => {
            if (type === "mention" || type === "hashtag") {
                return ""
            }
            return "_blank"
        },
        rel: (_, type) => {
            if (type === "mention" || type === "hashtag") {
                return ""
            }
            return "noopener noreferrer"
        },
        formatHref: {
            mention: href => "/@" + href.substr(1),
            hashtag: href => "/tagged-posts/" + href.substr(1),
        },
    })
}

/**
 * @param {string} s
 */
export function collectMediaURLs(s) {
    const out = []
    for (const item of findURLs(s)) {
        if (item.type === "url") {
            try {
                out.push(new URL(item.href, location.origin))
            } catch (_) {}
        }
    }
    return out
}
