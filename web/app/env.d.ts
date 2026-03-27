/// <reference types="vite/client" />

interface ImportMetaEnv {
    readonly VITE_VAPID_PUBLIC_KEY: string
    readonly VITE_MINIO_BASE_URL: string
}

interface ImportMeta {
    readonly env: ImportMetaEnv
}
