# use the official Bun image
# see all versions at https://hub.docker.com/r/oven/bun/tags
FROM node:24 AS base
WORKDIR /usr/src/app

RUN npm install -g bun

# install dependencies into temp directory
# this will cache them and speed up future builds
FROM base AS install
RUN mkdir -p /temp/dev
RUN mkdir -p /temp/dev/apps/web
RUN mkdir -p /temp/dev/packages/database
COPY apps/web/package.json /temp/dev/apps/web/package.json
COPY packages/database/package.json /temp/dev/packages/database/package.json
COPY package.json /temp/dev/package.json
# COPY bun.lock /temp/dev/bun.lock
RUN cd /temp/dev && bun install 

# copy node_modules from temp directory
# then copy all (non-ignored) project files into the image
FROM base AS prerelease
# Copy source files first
COPY ./apps/web ./apps/web
COPY ./packages/database ./packages/database
COPY ./package.json ./package.json
# Remove any node_modules that may have been copied (despite .dockerignore)
RUN rm -rf node_modules apps/web/node_modules packages/database/node_modules
# Then copy node_modules from install stage
COPY --from=install /temp/dev/node_modules node_modules
COPY --from=install /temp/dev/apps/web/node_modules apps/web/node_modules
# COPY --from=install /temp/dev/packages/database/node_modules packages/database/node_modules

# [optional] tests & build
# ENV NODE_ENV=production
# RUN bun test
RUN bun run web:build

# copy production dependencies and source code into final image
FROM base AS release
COPY --from=install /temp/dev/node_modules node_modules
COPY --from=install /temp/dev/apps/web/node_modules apps/web/node_modules
# COPY --from=install /temp/dev/packages/database/node_modules packages/database/node_modules
COPY --from=prerelease /usr/src/app/apps/web/.output .output

# run the app
USER node
EXPOSE 3000/tcp
ENTRYPOINT [ "node", ".output/server/index.mjs" ]
