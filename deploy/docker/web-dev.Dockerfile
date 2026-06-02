# SvelteKit dev server (Vite) with hot module reload.
#
# No build/copy here: the web source is bind-mounted at /app and node_modules
# lives on a named volume, so `pnpm install` + `pnpm dev` run from the compose
# `command:` once the mounts are in place. Mirrors the Node version used by the
# production web image.
FROM node:20-alpine

RUN corepack enable

WORKDIR /app

# Vite dev server (see .env.example: WEB_BASE_URL / dashboard run on :5173).
EXPOSE 5173
