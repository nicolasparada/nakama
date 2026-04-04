/** @type {import('vite').UserConfig} */
export default {
    publicDir: "../public",
    build: {
        outDir: "../dist",
    },
    server: {
        port: 4000,
        proxy: {
            "/api": {
                target: "http://localhost:3000",
                changeOrigin: true,
            },
        },
    },
}
