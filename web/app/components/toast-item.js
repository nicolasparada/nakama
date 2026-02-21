import { component, useEffect, useState } from "haunted"
import { html } from "lit"

/**
 * @typedef Toast
 * @property {string} type
 * @property {string} content
 * @property {number=} timeout
 */

function ToastItem({ toast: initialToast }) {
    const [toast, setToast] = useState(/** @type {Toast} */(Object.assign({ type: "", content: "", timeout: 5000 }, initialToast)))
    const [show, setShow] = useState(true)

    const onClick = () => {
        setShow(false)
    }

    useEffect(() => {
        const id = setTimeout(() => {
            setShow(false)
        }, toast.timeout)

        return () => {
            clearTimeout(id)
        }
    }, [toast.timeout])

    useEffect(() => {
        setToast(Object.assign({ type: "", content: "", timeout: 5000 }, initialToast))
        setShow(true)
    }, [initialToast])

    if (!show) {
        return null
    }

    return html`
        <div class="toast${toast.type !== "" ? " " + toast.type : ""}" role="status" aria-live="polite" @click=${onClick}>
            <p>${toast.content}</p>
        </div>
    `
}

// @ts-ignore
customElements.define("toast-item", component(ToastItem, { useShadowDOM: false }))
