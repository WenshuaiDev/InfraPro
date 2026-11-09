FROM node:26.10.0-alpine AS build
WORKDIR /src
RUN npm install --global pnpm@12.18.0
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/ts ./packages/ts
COPY apps ./apps
RUN pnpm install --frozen-lockfile
RUN VITE_APP_BASE=/console/ pnpm --filter @infrapro/console build

FROM nginx:1.28-alpine
COPY deploy/nginx/default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/console/dist /usr/share/nginx/html/console
EXPOSE 80
