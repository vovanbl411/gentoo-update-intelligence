import { defineConfig } from 'vite';
const apiProxyTarget = process.env.GENTOO_UPDATE_INTELLIGENCE_API_PROXY_TARGET ?? 'http://127.0.0.1:8080';
export default defineConfig({ server: { proxy: { '/api': { target: apiProxyTarget } } } });
